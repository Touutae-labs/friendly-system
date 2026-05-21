package processor_test

import (
	"context"
	"testing"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/modules/balance"
	"github.com/Touutae-labs/friendly-system/internal/modules/limit"
	"github.com/Touutae-labs/friendly-system/internal/modules/orderval"
	"github.com/Touutae-labs/friendly-system/internal/modules/quote"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/Touutae-labs/friendly-system/internal/service/processor"
	validatorsvc "github.com/Touutae-labs/friendly-system/internal/service/validator"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newStack(t *testing.T, db *gorm.DB, initialBalance string, initialTraded string) processor.ProcessorService {
	t.Helper()
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	if err := db.Create(&models.AccountModel{
		CustomerID: "C001",
		Balance:    dec(initialBalance),
	}).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}

	if initialTraded != "" {
		if err := db.Create(&models.DailyTotalModel{
			CustomerID: "C001",
			Day:        now.Format(time.DateOnly),
			Total:      dec(initialTraded),
		}).Error; err != nil {
			t.Fatalf("seed ledger: %v", err)
		}
	}

	ov, _ := orderval.New(orderval.DefaultConfig())

	mkt := quote.NewFixed(dec("1000"))
	q, _ := quote.New(quote.DefaultConfig(), mkt)

	accountRepo := repositories.NewAccount(db)
	b, _ := balance.New(accountRepo)

	ledger := repositories.NewLedger(db)
	limitCfg := limit.DefaultConfig()
	limitCfg.Clock = func() time.Time { return now }
	l, _ := limit.New(limitCfg, ledger)

	validator := validatorsvc.New(ov, q, b, l)

	orderRepo := repositories.NewOrder(db)
	return processor.New(db, validator, b, l, orderRepo, limitCfg)
}

func TestProcessor_HappyPath_Buy(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "5000", "")

	res := svc.Process(context.Background(), "key-001", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusFilled {
		t.Fatalf("expected filled, got %s (reason: %s)", res.Status, res.Reason)
	}
	if res.OrderID == "" {
		t.Error("expected non-empty order ID")
	}
	if res.NewBalance == nil || !res.NewBalance.Equal(dec("4000")) {
		t.Errorf("expected new balance 4000, got %v", res.NewBalance)
	}
	if res.DailyRemaining == nil || !res.DailyRemaining.Equal(dec("4")) {
		t.Errorf("expected daily remaining 4, got %v", res.DailyRemaining)
	}
}

func TestProcessor_HappyPath_Sell(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "1000", "")

	res := svc.Process(context.Background(), "key-sell-001", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Sell,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusFilled {
		t.Fatalf("expected filled, got %s", res.Status)
	}
	if res.NewBalance == nil || !res.NewBalance.Equal(dec("2000")) {
		t.Errorf("expected new balance 2000, got %v", res.NewBalance)
	}
}

func TestProcessor_Idempotency(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "5000", "")

	o := order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	}

	first := svc.Process(context.Background(), "key-idem-001", o)
	if first.Status != order.StatusFilled {
		t.Fatalf("first call: expected filled, got %s", first.Status)
	}

	second := svc.Process(context.Background(), "key-idem-001", o)
	if second.Status != order.StatusDuplicate {
		t.Fatalf("second call: expected duplicate, got %s", second.Status)
	}
	if second.OrderID != first.OrderID {
		t.Errorf("duplicate should return same order ID: got %s, want %s", second.OrderID, first.OrderID)
	}
}

func TestProcessor_Rejected_InsufficientBalance(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "500", "")

	res := svc.Process(context.Background(), "key-rej-bal", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusRejected {
		t.Fatalf("expected rejected, got %s", res.Status)
	}
}

func TestProcessor_Rejected_DailyLimitExceeded(t *testing.T) {
	db := newDB(t)
	// C001 has already traded 4.5 — limit is 5, so 1 more unit (1000 price) would push to 5.5
	svc := newStack(t, db, "100000", "4.5")

	res := svc.Process(context.Background(), "key-rej-limit", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusRejected {
		t.Fatalf("expected rejected (daily limit), got %s (reason: %s)", res.Status, res.Reason)
	}
}

func TestProcessor_Rejected_EmptyIdempotencyKey(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "5000", "")

	res := svc.Process(context.Background(), "", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusError {
		t.Fatalf("expected error status for empty key, got %s", res.Status)
	}
}

func TestProcessor_Rejected_CanceledContext(t *testing.T) {
	db := newDB(t)
	svc := newStack(t, db, "5000", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := svc.Process(ctx, "key-canceled", order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("1000"),
	})

	if res.Status != order.StatusError {
		t.Fatalf("expected error for canceled context, got %s", res.Status)
	}
}

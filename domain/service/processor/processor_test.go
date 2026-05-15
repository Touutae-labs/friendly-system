package processor_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/orderval"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
	"github.com/Touutae-labs/friendly-system/domain/repositories"
	"github.com/Touutae-labs/friendly-system/domain/service/processor"
	validatorsvc "github.com/Touutae-labs/friendly-system/domain/service/validator"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func setup(t *testing.T) (*processor.Service, *gorm.DB) {
	t.Helper()
	// :memory: with shared cache so concurrent connections see the same DB.
	// We keep one *gorm.DB alive for the test's lifetime, which keeps the
	// shared in-memory DB alive until the test ends.
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(repositories.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&[]repositories.AccountModel{
		{CustomerID: "C001", Balance: "1000000"},
		{CustomerID: "C002", Balance: "500"},
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	ov, _ := orderval.New(orderval.DefaultConfig())
	q, _ := quote.New(quote.DefaultConfig(), quote.NewMemory(dec("42000")))
	b, _ := balance.New(repositories.NewAccount(db))
	led, _ := limit.New(limit.DefaultConfig(), repositories.NewLedger(db))
	svc := validatorsvc.New(ov, q, b, led)

	proc := processor.New(db, svc, limit.DefaultConfig())
	return proc, db
}

func TestProcessor_FilledBuy(t *testing.T) {
	proc, db := setup(t)
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))

	r := proc.Process("idem-001", order.Order{
		CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expBuy,
	})

	if r.Status != processor.StatusFilled {
		t.Fatalf("status=%s, want filled (%+v)", r.Status, r)
	}
	if r.OrderID == "" {
		t.Error("expected order_id")
	}
	if r.NewBalance == nil {
		t.Fatal("expected new_balance")
	}

	var acct repositories.AccountModel
	if err := db.Where("customer_id = ?", "C001").First(&acct).Error; err != nil {
		t.Fatalf("account lookup: %v", err)
	}
	want := dec("1000000").Sub(dec("0.5").Mul(expBuy))
	got, _ := decimal.NewFromString(acct.Balance)
	if !got.Equal(want) {
		t.Errorf("balance after debit got %s, want %s", got, want)
	}

	var orderCount int64
	db.Model(&repositories.OrderModel{}).Count(&orderCount)
	if orderCount != 1 {
		t.Errorf("expected 1 audit row, got %d", orderCount)
	}
}

func TestProcessor_RejectsInsufficientBalance(t *testing.T) {
	proc, db := setup(t)
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))

	r := proc.Process("idem-002", order.Order{
		CustomerID: "C002", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expBuy,
	})

	if r.Status != processor.StatusRejected {
		t.Fatalf("status=%s, want rejected", r.Status)
	}

	var acct repositories.AccountModel
	db.Where("customer_id = ?", "C002").First(&acct)
	got, _ := decimal.NewFromString(acct.Balance)
	if !got.Equal(dec("500")) {
		t.Errorf("balance must not change on rejection, got %s, want 500", got)
	}
}

func TestProcessor_RejectsCustomerNotFound(t *testing.T) {
	proc, _ := setup(t)
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))

	r := proc.Process("idem-003", order.Order{
		CustomerID: "GHOST", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expBuy,
	})

	if r.Status != processor.StatusRejected {
		t.Fatalf("status=%s, want rejected", r.Status)
	}
	found := false
	for _, e := range r.ValidationErrors {
		if e.Code == balance.CodeCustomerNotFound {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s in errors, got %+v", balance.CodeCustomerNotFound, r.ValidationErrors)
	}
}

func TestProcessor_Idempotent(t *testing.T) {
	proc, db := setup(t)
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))
	o := order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expBuy}

	first := proc.Process("idem-dup", o)
	if first.Status != processor.StatusFilled {
		t.Fatalf("first status=%s", first.Status)
	}
	second := proc.Process("idem-dup", o)
	if second.Status != processor.StatusDuplicate {
		t.Fatalf("second status=%s, want duplicate", second.Status)
	}
	if second.OrderID != first.OrderID {
		t.Errorf("duplicate must return original order_id (%s != %s)", second.OrderID, first.OrderID)
	}

	var n int64
	db.Model(&repositories.OrderModel{}).Count(&n)
	if n != 1 {
		t.Errorf("expected exactly 1 audit row after duplicate POST, got %d", n)
	}

	var acct repositories.AccountModel
	db.Where("customer_id = ?", "C001").First(&acct)
	got, _ := decimal.NewFromString(acct.Balance)
	want := dec("1000000").Sub(dec("0.5").Mul(expBuy))
	if !got.Equal(want) {
		t.Errorf("balance must only debit ONCE, got %s, want %s", got, want)
	}
}

func TestProcessor_RejectsMissingIdempotencyKey(t *testing.T) {
	proc, _ := setup(t)
	r := proc.Process("", order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: dec("42210")})
	if r.Status != processor.StatusError {
		t.Errorf("status=%s, want error", r.Status)
	}
}

func TestProcessor_FilledSellCreditsBalance(t *testing.T) {
	proc, db := setup(t)
	r := proc.Process("idem-sell", order.Order{
		CustomerID: "C001", OrderType: order.Sell, Quantity: dec("0.5"), QuotedPrice: dec("42000"),
	})
	if r.Status != processor.StatusFilled {
		t.Fatalf("status=%s", r.Status)
	}

	var acct repositories.AccountModel
	db.Where("customer_id = ?", "C001").First(&acct)
	got, _ := decimal.NewFromString(acct.Balance)
	want := dec("1000000").Add(dec("0.5").Mul(dec("42000")))
	if !got.Equal(want) {
		t.Errorf("sell must credit balance, got %s, want %s", got, want)
	}
}

// Regression: 50 concurrent buys must never exceed the daily limit.
// The earlier processor relied on the validator's preflight (read without
// lock) and didn't re-check the limit inside the transaction, so all 50
// would fill instead of just enough to hit the limit. The lock + tx-level
// re-check fixed it.
func TestProcessor_ConcurrentBuysDoNotExceedLimit(t *testing.T) {
	proc, db := setup(t)
	// pre-load C001's daily total so headroom is 1 baht-weight (4 of 5 used);
	// each buy is 0.5 → only 2 should succeed.
	today := time.Now().UTC().Format("2006-01-02")
	if err := db.Create(&repositories.DailyTotalModel{CustomerID: "C001", Day: today, Total: "4"}).Error; err != nil {
		t.Fatalf("seed daily: %v", err)
	}
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))
	o := order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expBuy}

	const N = 50
	var wg sync.WaitGroup
	results := make(chan processor.Status, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := proc.Process(fmt.Sprintf("concurrent-%d", idx), o)
			results <- r.Status
		}(i)
	}
	wg.Wait()
	close(results)

	var filled, rejected int
	for s := range results {
		switch s {
		case processor.StatusFilled:
			filled++
		case processor.StatusRejected:
			rejected++
		}
	}
	if filled != 2 {
		t.Errorf("expected exactly 2 fills (headroom = 1, qty = 0.5), got %d filled / %d rejected", filled, rejected)
	}

	var dt repositories.DailyTotalModel
	if err := db.Where("customer_id = ? AND day = ?", "C001", today).First(&dt).Error; err != nil {
		t.Fatalf("read daily total: %v", err)
	}
	total, _ := decimal.NewFromString(dt.Total)
	if total.GreaterThan(dec("5")) {
		t.Errorf("daily_total %s exceeds limit 5 — TOCTOU bug regressed", total)
	}
}

func TestProcessor_DailyTotalIncrements(t *testing.T) {
	proc, db := setup(t)
	expBuy := dec("42000").Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))

	for i, key := range []string{"a", "b"} {
		r := proc.Process(key, order.Order{
			CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expBuy,
		})
		if r.Status != processor.StatusFilled {
			t.Fatalf("buy %d status=%s", i, r.Status)
		}
	}

	day := time.Now().UTC().Format("2006-01-02")
	var dt repositories.DailyTotalModel
	if err := db.Where("customer_id = ? AND day = ?", "C001", day).First(&dt).Error; err != nil {
		t.Fatalf("daily total: %v", err)
	}
	total, _ := decimal.NewFromString(dt.Total)
	if !total.Equal(dec("1")) {
		t.Errorf("expected daily total 1, got %s", total)
	}
}

package validatorsvc_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/orderval"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
	validatorsvc "github.com/Touutae-labs/friendly-system/domain/service/validator"
	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	}
}

func mustNew(t *testing.T, accounts balance.AccountRepository, mkt quote.MarketPriceProvider, led limit.DailyLedger) *validatorsvc.Service {
	t.Helper()
	ov, err := orderval.New(orderval.DefaultConfig())
	if err != nil {
		t.Fatalf("orderval.New: %v", err)
	}
	q, err := quote.New(quote.DefaultConfig(), mkt)
	if err != nil {
		t.Fatalf("quote.New: %v", err)
	}
	b, err := balance.New(accounts)
	if err != nil {
		t.Fatalf("balance.New: %v", err)
	}
	var l *limit.Validator
	if led != nil {
		l, err = limit.New(limit.DefaultConfig(), led)
		if err != nil {
			t.Fatalf("limit.New: %v", err)
		}
		l = l.WithClock(fixedClock())
	}
	return validatorsvc.New(ov, q, b, l)
}

func fixtures(t *testing.T, marketPrice string) (*validatorsvc.Service, *limit.Memory) {
	t.Helper()
	accounts := balance.NewMemory(map[string]decimal.Decimal{
		"C001": dec("1000000"),
		"C002": dec("500"),
	})
	mkt := quote.NewMemory(dec(marketPrice))
	led := limit.NewMemory()
	return mustNew(t, accounts, mkt, led), led
}

func expectedBuyPrice(marketPrice string) decimal.Decimal {
	m := dec(marketPrice)
	return m.Mul(decimal.NewFromInt(1).Add(quote.DefaultConfig().SpreadMargin))
}

func TestInvalidOrderType(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Type("HODL"), Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidOrderType) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidOrderType, r.Errors)
	}
}

func TestNegativeQuantity(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("-1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidQuantity) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidQuantity, r.Errors)
	}
}

func TestZeroQuantity(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidQuantity) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidQuantity, r.Errors)
	}
}

func TestQuantityNotMultipleOfIncrement(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.7"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidQuantityIncr) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidQuantityIncr, r.Errors)
	}
}

func TestQuantityValidIncrements(t *testing.T) {
	v, _ := fixtures(t, "42000")
	for _, q := range []string{"0.5", "1", "1.5", "2.5", "5"} {
		r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec(q), QuotedPrice: expectedBuyPrice("42000")})
		if r.HasError(orderval.CodeInvalidQuantityIncr) {
			t.Fatalf("quantity %s should be a valid increment, got %+v", q, r.Errors)
		}
	}
}

func TestZeroPriceRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: dec("0")})
	if !r.HasError(orderval.CodeInvalidPrice) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidPrice, r.Errors)
	}
}

func TestNegativePriceRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: dec("-5")})
	if !r.HasError(orderval.CodeInvalidPrice) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidPrice, r.Errors)
	}
}

func TestMissingCustomerID(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeMissingCustomerID) {
		t.Fatalf("expected %s, got %+v", orderval.CodeMissingCustomerID, r.Errors)
	}
}

func TestMultipleErrorsCollectedAtOnce(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "", OrderType: order.Type("foo"), Quantity: dec("0"), QuotedPrice: dec("0")})
	codes := map[string]bool{}
	for _, e := range r.Errors {
		codes[e.Code] = true
	}
	for _, c := range []string{orderval.CodeInvalidOrderType, orderval.CodeInvalidQuantity, orderval.CodeInvalidPrice, orderval.CodeMissingCustomerID} {
		if !codes[c] {
			t.Errorf("expected %s, got %+v", c, r.Errors)
		}
	}
}

func TestBuyInsufficientBalance(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C002", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(balance.CodeInsufficientBalance) {
		t.Fatalf("expected %s, got %+v", balance.CodeInsufficientBalance, r.Errors)
	}
}

func TestStalePriceForSell(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("1"), QuotedPrice: dec("38000")})
	if !r.HasError(quote.CodeStalePrice) {
		t.Fatalf("expected %s, got %+v", quote.CodeStalePrice, r.Errors)
	}
}

func TestSellWithinToleranceAccepted(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("1"), QuotedPrice: dec("41500")})
	if !r.Valid {
		t.Fatalf("expected valid, got %+v", r.Errors)
	}
}

func TestCustomerNotFoundOnBuy(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "GHOST", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(balance.CodeCustomerNotFound) {
		t.Fatalf("expected %s, got %+v", balance.CodeCustomerNotFound, r.Errors)
	}
}

func TestMarketUnavailableFailsClosed(t *testing.T) {
	accounts := balance.NewMemory(map[string]decimal.Decimal{"C001": dec("1000000")})
	v := mustNew(t, accounts, quote.NewMemory(dec("0")), nil)
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: dec("42000")})
	if !r.HasError(quote.CodeMarketUnavailable) {
		t.Fatalf("expected %s, got %+v", quote.CodeMarketUnavailable, r.Errors)
	}
}

func TestSpreadIsCalculatedForBuy(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.Valid {
		t.Fatalf("expected valid, got %+v", r.Errors)
	}
	if r.ExpectedBuyPrice == nil || r.Spread == nil {
		t.Fatal("expected ExpectedBuyPrice and Spread populated for buys")
	}
	if !r.Spread.Equal(dec("210")) {
		t.Fatalf("expected spread 210, got %s", r.Spread.String())
	}
	if !r.ExpectedBuyPrice.Equal(dec("42210")) {
		t.Fatalf("expected buy price 42210, got %s", r.ExpectedBuyPrice.String())
	}
}

func TestQuotedBuyPriceWayOffRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: dec("38000")})
	if !r.HasError(quote.CodeStaleOrOffPrice) {
		t.Fatalf("expected %s, got %+v", quote.CodeStaleOrOffPrice, r.Errors)
	}
}

func TestDailyLimitExceededByCurrentOrderAlone(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("5.5"), QuotedPrice: dec("42000")})
	if !r.HasError(limit.CodeDailyLimitExceeded) {
		t.Fatalf("expected %s, got %+v", limit.CodeDailyLimitExceeded, r.Errors)
	}
	if r.DailyRemaining == nil || !r.DailyRemaining.Equal(dec("5")) {
		t.Fatalf("expected DailyRemaining=5, got %v", r.DailyRemaining)
	}
}

func TestDailyLimitExceededAfterPriorOrders(t *testing.T) {
	v, led := fixtures(t, "42000")
	led.Record("C001", dec("4.5"), time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC))
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("1"), QuotedPrice: dec("42000")})
	if !r.HasError(limit.CodeDailyLimitExceeded) {
		t.Fatalf("expected %s, got %+v", limit.CodeDailyLimitExceeded, r.Errors)
	}
	if r.DailyRemaining == nil || !r.DailyRemaining.Equal(dec("0.5")) {
		t.Fatalf("expected DailyRemaining=0.5, got %v", r.DailyRemaining)
	}
}

func TestDailyLimitExactlyAtBoundaryAllowed(t *testing.T) {
	v, led := fixtures(t, "42000")
	led.Record("C001", dec("4.5"), time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC))
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("0.5"), QuotedPrice: dec("42000")})
	if !r.Valid {
		t.Fatalf("expected valid (at boundary), got %+v", r.Errors)
	}
	if r.DailyRemaining == nil || !r.DailyRemaining.Equal(dec("0")) {
		t.Fatalf("expected DailyRemaining=0, got %v", r.DailyRemaining)
	}
}

func TestValidBuyHappyPath(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.Valid {
		t.Fatalf("expected valid, got %+v", r.Errors)
	}
	if r.DailyRemaining == nil || !r.DailyRemaining.Equal(dec("4")) {
		t.Fatalf("expected DailyRemaining=4, got %v", r.DailyRemaining)
	}
}

func TestValidSellHappyPath(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("2.5"), QuotedPrice: dec("42000")})
	if !r.Valid {
		t.Fatalf("expected valid, got %+v", r.Errors)
	}
	if r.Spread != nil || r.ExpectedBuyPrice != nil {
		t.Errorf("spread/expected buy should be nil on sell, got %+v / %+v", r.Spread, r.ExpectedBuyPrice)
	}
}

func TestQuantityAtMaxAllowed(t *testing.T) {
	v, _ := fixtures(t, "42000")
	cfg := orderval.DefaultConfig()
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: cfg.MaxQuantity, QuotedPrice: dec("42000")})
	if r.HasError(orderval.CodeQuantityTooLarge) {
		t.Fatalf("at-boundary quantity should not trigger %s, got %+v", orderval.CodeQuantityTooLarge, r.Errors)
	}
}

func TestQuantityOneIncrementOverMaxRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	cfg := orderval.DefaultConfig()
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: cfg.MaxQuantity.Add(cfg.QuantityIncrement), QuotedPrice: dec("42000")})
	if !r.HasError(orderval.CodeQuantityTooLarge) {
		t.Fatalf("expected %s, got %+v", orderval.CodeQuantityTooLarge, r.Errors)
	}
}

func TestPriceOverMaxRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: dec("99999999")})
	if !r.HasError(orderval.CodePriceTooLarge) {
		t.Fatalf("expected %s, got %+v", orderval.CodePriceTooLarge, r.Errors)
	}
}

func TestCustomerIDAtMaxLengthAllowed(t *testing.T) {
	v, _ := fixtures(t, "42000")
	cfg := orderval.DefaultConfig()
	id := strings.Repeat("a", cfg.MaxCustomerIDLength)
	r := v.Validate(order.Order{CustomerID: id, OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if r.HasError(orderval.CodeInvalidCustomerID) {
		t.Fatalf("at-boundary ID should not be invalid, got %+v", r.Errors)
	}
}

func TestCustomerIDOverMaxLengthRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	cfg := orderval.DefaultConfig()
	id := strings.Repeat("a", cfg.MaxCustomerIDLength+1)
	r := v.Validate(order.Order{CustomerID: id, OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidCustomerID) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidCustomerID, r.Errors)
	}
}

func TestCustomerIDWithLeadingWhitespaceRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: " C001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidCustomerID) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidCustomerID, r.Errors)
	}
}

func TestCustomerIDWithControlCharRejected(t *testing.T) {
	v, _ := fixtures(t, "42000")
	r := v.Validate(order.Order{CustomerID: "C\x00001", OrderType: order.Buy, Quantity: dec("1"), QuotedPrice: expectedBuyPrice("42000")})
	if !r.HasError(orderval.CodeInvalidCustomerID) {
		t.Fatalf("expected %s, got %+v", orderval.CodeInvalidCustomerID, r.Errors)
	}
}

func TestValidator_ConcurrentValidate(t *testing.T) {
	v, _ := fixtures(t, "42000")
	const goroutines = 50
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_ = v.Validate(order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expectedBuyPrice("42000")})
			}
		}()
	}
	wg.Wait()
}

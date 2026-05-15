package validatorsvc_test

import (
	"testing"
	"time"

	"github.com/pantakan/intergold-validator/domain/common/order"
	"github.com/pantakan/intergold-validator/domain/module/balance"
	"github.com/pantakan/intergold-validator/domain/module/limit"
	"github.com/pantakan/intergold-validator/domain/module/orderval"
	"github.com/pantakan/intergold-validator/domain/module/quote"
	validatorsvc "github.com/pantakan/intergold-validator/domain/service/validator"
	"github.com/shopspring/decimal"
)

func newBenchService(b *testing.B, accounts balance.AccountRepository, mkt quote.MarketPriceProvider, led limit.DailyLedger) *validatorsvc.Service {
	b.Helper()
	ov, err := orderval.New(orderval.DefaultConfig())
	if err != nil {
		b.Fatalf("orderval.New: %v", err)
	}
	q, err := quote.New(quote.DefaultConfig(), mkt)
	if err != nil {
		b.Fatalf("quote.New: %v", err)
	}
	ba, err := balance.New(accounts)
	if err != nil {
		b.Fatalf("balance.New: %v", err)
	}
	var l *limit.Validator
	if led != nil {
		l, err = limit.New(limit.DefaultConfig(), led)
		if err != nil {
			b.Fatalf("limit.New: %v", err)
		}
		l = l.WithClock(func() time.Time {
			return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
		})
	}
	return validatorsvc.New(ov, q, ba, l)
}

func BenchmarkValidate_HappyBuy(b *testing.B) {
	accounts := balance.NewMemory(map[string]decimal.Decimal{"C001": dec("1000000")})
	v := newBenchService(b, accounts, quote.NewMemory(dec("42000")), limit.NewMemory())
	o := order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: dec("0.5"), QuotedPrice: expectedBuyPrice("42000")}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Validate(o)
	}
}

func BenchmarkValidate_RejectedDailyLimit(b *testing.B) {
	accounts := balance.NewMemory(map[string]decimal.Decimal{"C001": dec("1000000")})
	led := limit.NewMemory()
	led.Record("C001", dec("4.5"), time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))
	v := newBenchService(b, accounts, quote.NewMemory(dec("42000")), led)
	o := order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: dec("1"), QuotedPrice: dec("42000")}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Validate(o)
	}
}

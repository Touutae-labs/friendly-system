package validatorsvc_test

import (
	"context"
	"testing"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/Touutae-labs/friendly-system/internal/modules/quote"
	"github.com/Touutae-labs/friendly-system/internal/testhelpers"
	"github.com/shopspring/decimal"
)

func BenchmarkValidate_ValidBuy(b *testing.B) {
	accounts := testhelpers.NewAccountMemory(map[string]decimal.Decimal{
		"C001": decimal.RequireFromString("1000000"),
	})
	led := testhelpers.NewLedgerMemory()
	svc := mustNew(b, accounts, quote.NewFixed(decimal.RequireFromString("1000")), led)

	o := order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    decimal.RequireFromString("1"),
		QuotedPrice: decimal.RequireFromString("1000"),
	}

	b.ResetTimer()
	for range b.N {
		svc.Validate(context.Background(), o)
	}
}

func BenchmarkValidate_FailFastInvalidOrder(b *testing.B) {
	accounts := testhelpers.NewAccountMemory(map[string]decimal.Decimal{
		"C001": decimal.RequireFromString("1000000"),
	})
	led := testhelpers.NewLedgerMemory()
	svc := mustNew(b, accounts, quote.NewFixed(decimal.RequireFromString("1000")), led)

	// zero quantity — rejected by orderval before any I/O
	o := order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    decimal.Zero,
		QuotedPrice: decimal.RequireFromString("1000"),
	}

	b.ResetTimer()
	for range b.N {
		svc.Validate(context.Background(), o)
	}
}

func BenchmarkValidate_Parallel(b *testing.B) {
	accounts := testhelpers.NewAccountMemory(map[string]decimal.Decimal{
		"C001": decimal.RequireFromString("1000000"),
	})
	led := testhelpers.NewLedgerMemory()
	svc := mustNew(b, accounts, quote.NewFixed(decimal.RequireFromString("1000")), led)

	o := order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    decimal.RequireFromString("0.5"),
		QuotedPrice: decimal.RequireFromString("1000"),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			svc.Validate(context.Background(), o)
		}
	})
}

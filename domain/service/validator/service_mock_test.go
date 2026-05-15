package validatorsvc_test

import (
	"errors"
	"testing"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	balancemocks "github.com/Touutae-labs/friendly-system/domain/mocks/balance"
	limitmocks "github.com/Touutae-labs/friendly-system/domain/mocks/limit"
	quotemocks "github.com/Touutae-labs/friendly-system/domain/mocks/quote"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
)

func TestService_AccountLookupFailure_UsingMocks(t *testing.T) {
	accounts := balancemocks.NewAccountRepository(t)
	mkt := quotemocks.NewMarketPriceProvider(t)

	mkt.EXPECT().CurrentPrice().Return(dec("42000"), nil)
	accounts.EXPECT().Balance("C001").Return(decimal.Zero, errors.New("db unavailable"))

	v := mustNew(t, accounts, mkt, nil)

	r := v.Validate(order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("42210"),
	})

	if !r.HasError(balance.CodeAccountLookupFailed) {
		t.Fatalf("expected %s, got %+v", balance.CodeAccountLookupFailed, r.Errors)
	}
}

func TestService_LedgerLookupFailure_UsingMocks(t *testing.T) {
	accounts := balancemocks.NewAccountRepository(t)
	mkt := quotemocks.NewMarketPriceProvider(t)
	led := limitmocks.NewDailyLedger(t)

	mkt.EXPECT().CurrentPrice().Return(dec("42000"), nil)
	accounts.EXPECT().Balance("C001").Return(dec("1000000"), nil)
	led.EXPECT().
		TradedToday("C001", mock.Anything).
		Return(decimal.Zero, errors.New("redis timeout"))

	v := mustNew(t, accounts, mkt, led)

	r := v.Validate(order.Order{
		CustomerID:  "C001",
		OrderType:   order.Buy,
		Quantity:    dec("1"),
		QuotedPrice: dec("42210"),
	})

	if !r.HasError(limit.CodeLedgerLookupFailed) {
		t.Fatalf("expected %s, got %+v", limit.CodeLedgerLookupFailed, r.Errors)
	}
}

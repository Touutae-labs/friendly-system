package wire

import (
	"time"

	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
	"github.com/shopspring/decimal"
)

func provideAccounts() balance.AccountRepository {
	return balance.NewMemory(map[string]decimal.Decimal{
		"C001": decimal.RequireFromString("1000000"),
		"C002": decimal.RequireFromString("500"),
	})
}

func provideMarket() quote.MarketPriceProvider {
	return quote.NewMemory(decimal.RequireFromString("42000"))
}

func provideLedger() limit.DailyLedger {
	m := limit.NewMemory()
	m.Record("C001", decimal.RequireFromString("4"), time.Now())
	return m
}

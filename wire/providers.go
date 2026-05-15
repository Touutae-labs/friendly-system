package wire

import (
	"time"

	"github.com/pantakan/intergold-validator/domain/module/balance"
	"github.com/pantakan/intergold-validator/domain/module/limit"
	"github.com/pantakan/intergold-validator/domain/module/quote"
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

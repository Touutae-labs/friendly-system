package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/pantakan/intergold-validator/domain/common/order"
	"github.com/pantakan/intergold-validator/domain/module/quote"
	validatorsvc "github.com/pantakan/intergold-validator/domain/service/validator"
	"github.com/pantakan/intergold-validator/wire"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func main() {
	dbPath := flag.String("db", "", "path to SQLite database (omit → in-memory adapters)")
	flag.Parse()

	svc, cleanup, err := buildService(*dbPath)
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	defer cleanup()

	market := decimal.RequireFromString("42000")
	qcfg := quote.DefaultConfig()
	expectedBuy := market.Mul(decimal.NewFromInt(1).Add(qcfg.SpreadMargin))

	cases := []struct {
		label string
		order order.Order
	}{
		{
			label: "valid 0.5 buy from C001",
			order: order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: decimal.RequireFromString("0.5"), QuotedPrice: expectedBuy},
		},
		{
			label: "buy that pushes C001 over the daily limit",
			order: order.Order{CustomerID: "C001", OrderType: order.Buy, Quantity: decimal.RequireFromString("1.5"), QuotedPrice: expectedBuy},
		},
		{
			label: "buy with insufficient balance (C002 has 500 THB)",
			order: order.Order{CustomerID: "C002", OrderType: order.Buy, Quantity: decimal.RequireFromString("1"), QuotedPrice: expectedBuy},
		},
		{
			label: "sell with stale price",
			order: order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: decimal.RequireFromString("1"), QuotedPrice: decimal.RequireFromString("38000")},
		},
		{
			label: "completely malformed order (collects all errors at once)",
			order: order.Order{CustomerID: "", OrderType: order.Type("HODL"), Quantity: decimal.RequireFromString("0.7"), QuotedPrice: decimal.RequireFromString("-1")},
		},
		{
			label: "customer_id with embedded NUL byte (hardening)",
			order: order.Order{CustomerID: "C\x00001", OrderType: order.Buy, Quantity: decimal.RequireFromString("1"), QuotedPrice: expectedBuy},
		},
		{
			label: "quantity over MaxQuantity ceiling (hardening)",
			order: order.Order{CustomerID: "C001", OrderType: order.Sell, Quantity: decimal.RequireFromString("9999.5"), QuotedPrice: decimal.RequireFromString("42000")},
		},
	}

	bar := strings.Repeat("-", 72)
	for i, c := range cases {
		fmt.Println(bar)
		fmt.Printf("Case %d: %s\n", i+1, c.label)
		fmt.Printf("  Order: %+v\n", c.order)
		r := svc.Validate(c.order)
		fmt.Printf("  Valid: %v\n", r.Valid)
		if r.ExpectedBuyPrice != nil {
			fmt.Printf("  Expected buy price: %s THB\n", r.ExpectedBuyPrice.StringFixed(2))
		}
		if r.Spread != nil {
			fmt.Printf("  Spread: %s THB / baht-weight\n", r.Spread.StringFixed(2))
		}
		if r.DailyRemaining != nil {
			fmt.Printf("  Daily allowance remaining: %s baht-weight\n", r.DailyRemaining.String())
		}
		for _, e := range r.Errors {
			fmt.Printf("  [%s] %s\n", e.Code, e.Message)
		}
	}
	fmt.Println(bar)
}

func buildService(dbPath string) (*validatorsvc.Service, func(), error) {
	if dbPath == "" {
		svc, err := wire.InitValidatorService()
		return svc, func() {}, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	svc, err := wire.InitValidatorServiceGorm(db)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}
	return svc, cleanup, nil
}

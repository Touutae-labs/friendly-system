package repositories

import (
	"time"

	"github.com/shopspring/decimal"
)

type AccountModel struct {
	CustomerID string          `gorm:"primaryKey;column:customer_id"`
	Balance    decimal.Decimal `gorm:"column:balance;type:text;not null"`
}

func (AccountModel) TableName() string { return "accounts" }

type DailyTotalModel struct {
	CustomerID string          `gorm:"primaryKey;column:customer_id"`
	Day        string          `gorm:"primaryKey;column:day"`
	Total      decimal.Decimal `gorm:"column:total;type:text;not null"`
}

func (DailyTotalModel) TableName() string { return "daily_totals" }

type OrderModel struct {
	ID             string          `gorm:"primaryKey;column:id"`
	CustomerID     string          `gorm:"column:customer_id;not null;index"`
	OrderType      string          `gorm:"column:order_type;not null"`
	Quantity       decimal.Decimal `gorm:"column:quantity;type:text;not null"`
	QuotedPrice    decimal.Decimal `gorm:"column:quoted_price;type:text;not null"`
	Total          decimal.Decimal `gorm:"column:total;type:text;not null"`
	NewBalance     decimal.Decimal `gorm:"column:new_balance;type:text;not null"`
	IdempotencyKey string          `gorm:"column:idempotency_key;not null;uniqueIndex"`
	CreatedAt      time.Time       `gorm:"column:created_at;not null"`
}

func (OrderModel) TableName() string { return "orders" }

func AllModels() []any {
	return []any{&AccountModel{}, &DailyTotalModel{}, &OrderModel{}}
}

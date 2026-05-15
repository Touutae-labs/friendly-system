package repositories

type AccountModel struct {
	CustomerID string `gorm:"primaryKey;column:customer_id"`
	Balance    string `gorm:"column:balance;not null"`
}

func (AccountModel) TableName() string { return "accounts" }

type DailyTotalModel struct {
	CustomerID string `gorm:"primaryKey;column:customer_id"`
	Day        string `gorm:"primaryKey;column:day"`
	Total      string `gorm:"column:total;not null"`
}

func (DailyTotalModel) TableName() string { return "daily_totals" }

type OrderModel struct {
	ID             string `gorm:"primaryKey;column:id"`
	CustomerID     string `gorm:"column:customer_id;not null;index"`
	OrderType      string `gorm:"column:order_type;not null"`
	Quantity       string `gorm:"column:quantity;not null"`
	QuotedPrice    string `gorm:"column:quoted_price;not null"`
	Total          string `gorm:"column:total;not null"`
	NewBalance     string `gorm:"column:new_balance;not null"`
	IdempotencyKey string `gorm:"column:idempotency_key;not null;uniqueIndex"`
	CreatedAt      string `gorm:"column:created_at;not null"`
}

func (OrderModel) TableName() string { return "orders" }

func AllModels() []any {
	return []any{&AccountModel{}, &DailyTotalModel{}, &OrderModel{}}
}

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

func AllModels() []any {
	return []any{&AccountModel{}, &DailyTotalModel{}}
}

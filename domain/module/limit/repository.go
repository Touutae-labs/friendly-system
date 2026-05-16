package limit

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type DailyLedger interface {
	TradedToday(ctx context.Context, customerID string, now time.Time) (decimal.Decimal, error)
	LockAndGetTotal(ctx context.Context, tx *gorm.DB, customerID string, day string) (decimal.Decimal, error)
	UpsertTotal(ctx context.Context, tx *gorm.DB, customerID string, day string, newTotal decimal.Decimal) error
}

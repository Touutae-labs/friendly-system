package limit

import (
	"time"

	"github.com/shopspring/decimal"
)

type DailyLedger interface {
	TradedToday(customerID string, now time.Time) (decimal.Decimal, error)
}

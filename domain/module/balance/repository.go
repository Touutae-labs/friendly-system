package balance

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrCustomerNotFound = errors.New("customer not found")

type AccountRepository interface {
	Balance(ctx context.Context, customerID string) (decimal.Decimal, error)
	LockAndGetBalance(ctx context.Context, tx *gorm.DB, customerID string) (decimal.Decimal, error)
	UpdateBalance(ctx context.Context, tx *gorm.DB, customerID string, newBalance decimal.Decimal) error
}

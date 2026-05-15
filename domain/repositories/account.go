package repositories

import (
	"errors"
	"fmt"

	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var _ balance.AccountRepository = (*Account)(nil)

type Account struct {
	db *gorm.DB
}

func NewAccount(db *gorm.DB) *Account {
	return &Account{db: db}
}

func (a *Account) Balance(customerID string) (decimal.Decimal, error) {
	var row AccountModel
	err := a.db.Where("customer_id = ?", customerID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, balance.ErrCustomerNotFound
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Account: query: %w", err)
	}
	bal, err := decimal.NewFromString(row.Balance)
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Account: parse %q: %w", row.Balance, err)
	}
	return bal, nil
}

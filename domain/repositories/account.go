package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ balance.AccountRepository = (*Account)(nil)

type Account struct {
	db *gorm.DB
}

func NewAccount(db *gorm.DB) *Account {
	return &Account{db: db}
}

func (a *Account) Balance(ctx context.Context, customerID string) (decimal.Decimal, error) {
	return a.query(ctx, a.db, customerID)
}

func (a *Account) LockAndGetBalance(ctx context.Context, tx *gorm.DB, customerID string) (decimal.Decimal, error) {
	return a.query(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), customerID)
}

func (a *Account) query(ctx context.Context, db *gorm.DB, customerID string) (decimal.Decimal, error) {
	var row AccountModel
	err := db.WithContext(ctx).Where("customer_id = ?", customerID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, balance.ErrCustomerNotFound
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Account: query: %w", err)
	}
	return row.Balance, nil
}

func (a *Account) UpdateBalance(ctx context.Context, tx *gorm.DB, customerID string, newBalance decimal.Decimal) error {
	return tx.WithContext(ctx).Model(&AccountModel{}).
		Where("customer_id = ?", customerID).
		Update("balance", newBalance).Error
}

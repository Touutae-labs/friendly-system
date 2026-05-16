package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ limit.DailyLedger = (*Ledger)(nil)

type Ledger struct {
	db *gorm.DB
}

func NewLedger(db *gorm.DB) *Ledger {
	return &Ledger{db: db}
}

func (l *Ledger) TradedToday(ctx context.Context, customerID string, now time.Time) (decimal.Decimal, error) {
	day := now.UTC().Format(time.DateOnly)
	var row DailyTotalModel
	err := l.db.WithContext(ctx).Where("customer_id = ? AND day = ?", customerID, day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: query: %w", err)
	}
	return row.Total, nil
}

func (l *Ledger) LockAndGetTotal(ctx context.Context, tx *gorm.DB, customerID string, day string) (decimal.Decimal, error) {
	var row DailyTotalModel
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("customer_id = ? AND day = ?", customerID, day).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: lock: %w", err)
	}
	return row.Total, nil
}

func (l *Ledger) UpsertTotal(ctx context.Context, tx *gorm.DB, customerID string, day string, newTotal decimal.Decimal) error {
	return tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"total"}),
	}).Create(&DailyTotalModel{
		CustomerID: customerID,
		Day:        day,
		Total:      newTotal,
	}).Error
}

func (l *Ledger) Record(ctx context.Context, customerID string, qty decimal.Decimal, now time.Time) error {
	day := now.UTC().Format(time.DateOnly)

	return l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := l.LockAndGetTotal(ctx, tx, customerID, day)
		if err != nil {
			return err
		}
		return l.UpsertTotal(ctx, tx, customerID, day, current.Add(qty))
	})
}

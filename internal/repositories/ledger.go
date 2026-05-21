package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/txutil"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DailyLedger interface {
	TradedToday(ctx context.Context, customerID string, now time.Time) (decimal.Decimal, error)
	LockAndGetTotal(ctx context.Context, customerID string, day string) (decimal.Decimal, error)
	UpsertTotal(ctx context.Context, customerID string, day string, newTotal decimal.Decimal) error
}

var _ DailyLedger = (*Ledger)(nil)

type Ledger struct {
	db *gorm.DB
}

func NewLedger(db *gorm.DB) *Ledger {
	return &Ledger{db: db}
}

func (l *Ledger) TradedToday(ctx context.Context, customerID string, now time.Time) (decimal.Decimal, error) {
	day := now.UTC().Format(time.DateOnly)
	var row models.DailyTotalModel
	err := l.db.WithContext(ctx).Where("customer_id = ? AND day = ?", customerID, day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: query: %w", err)
	}
	return row.Total, nil
}

func (l *Ledger) LockAndGetTotal(ctx context.Context, customerID string, day string) (decimal.Decimal, error) {
	db := txutil.FromContext(ctx, l.db)
	var row models.DailyTotalModel
	err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
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

func (l *Ledger) UpsertTotal(ctx context.Context, customerID string, day string, newTotal decimal.Decimal) error {
	db := txutil.FromContext(ctx, l.db)
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"total"}),
	}).Create(&models.DailyTotalModel{
		CustomerID: customerID,
		Day:        day,
		Total:      newTotal,
	}).Error
}

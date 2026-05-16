package repositories

import (
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

func (l *Ledger) TradedToday(customerID string, now time.Time) (decimal.Decimal, error) {
	day := now.UTC().Format("2006-01-02")
	var row DailyTotalModel
	err := l.db.Where("customer_id = ? AND day = ?", customerID, day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: query: %w", err)
	}
	return row.Total, nil
}

func (l *Ledger) Record(customerID string, qty decimal.Decimal, now time.Time) error {
	day := now.UTC().Format("2006-01-02")

	return l.db.Transaction(func(tx *gorm.DB) error {
		var row DailyTotalModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ? AND day = ?", customerID, day).
			First(&row).Error

		current := decimal.Zero
		if err == nil {
			current = row.Total
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("repositories.Ledger: read: %w", err)
		}

		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
			DoUpdates: clause.AssignmentColumns([]string{"total"}),
		}).Create(&DailyTotalModel{
			CustomerID: customerID,
			Day:        day,
			Total:      current.Add(qty),
		}).Error
	})
}

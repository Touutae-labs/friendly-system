package repositories

import (
	"errors"
	"fmt"
	"time"

	"github.com/pantakan/intergold-validator/domain/module/limit"
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
	day := now.Format("2006-01-02")
	var row DailyTotalModel
	err := l.db.Where("customer_id = ? AND day = ?", customerID, day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, nil
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: query: %w", err)
	}
	total, err := decimal.NewFromString(row.Total)
	if err != nil {
		return decimal.Zero, fmt.Errorf("repositories.Ledger: parse %q: %w", row.Total, err)
	}
	return total, nil
}

// Record performs an atomic read-modify-write on daily_totals inside a GORM
// transaction. We rely on `BEGIN IMMEDIATE` (set via the connection-level
// transaction lock for SQLite) so two concurrent Records for the same
// (customer, day) cannot both read the same "current" and produce a lost
// update. For Postgres / MySQL the equivalent guarantee comes from the
// FOR UPDATE row lock taken by GORM's transaction when used with serialisable
// isolation; the caller wires the isolation level when opening the DB.
func (l *Ledger) Record(customerID string, qty decimal.Decimal, now time.Time) error {
	day := now.Format("2006-01-02")

	return l.db.Transaction(func(tx *gorm.DB) error {
		var row DailyTotalModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ? AND day = ?", customerID, day).
			First(&row).Error

		current := decimal.Zero
		if err == nil {
			current, err = decimal.NewFromString(row.Total)
			if err != nil {
				return fmt.Errorf("repositories.Ledger: parse: %w", err)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("repositories.Ledger: read: %w", err)
		}

		next := current.Add(qty)
		upsert := DailyTotalModel{
			CustomerID: customerID,
			Day:        day,
			Total:      next.String(),
		}

		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
			DoUpdates: clause.AssignmentColumns([]string{"total"}),
		}).Create(&upsert).Error
	})
}

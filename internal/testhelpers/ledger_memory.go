package testhelpers

import (
	"context"
	"sync"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/shopspring/decimal"
)

var _ repositories.DailyLedger = (*LedgerMemory)(nil)

type dayKey struct {
	customerID string
	day        string
}

type LedgerMemory struct {
	mu         sync.Mutex
	totals     map[dayKey]decimal.Decimal
	forceError error
}

func NewLedgerMemory() *LedgerMemory {
	return &LedgerMemory{totals: map[dayKey]decimal.Decimal{}}
}

func (m *LedgerMemory) WithError(err error) *LedgerMemory {
	m.forceError = err
	return m
}

func (m *LedgerMemory) Record(customerID string, qty decimal.Decimal, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := dayKey{customerID, now.UTC().Format(time.DateOnly)}
	m.totals[k] = m.totals[k].Add(qty)
}

func (m *LedgerMemory) TradedToday(_ context.Context, customerID string, now time.Time) (decimal.Decimal, error) {
	if m.forceError != nil {
		return decimal.Zero, m.forceError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := dayKey{customerID, now.UTC().Format(time.DateOnly)}
	return m.totals[k], nil
}

func (m *LedgerMemory) LockAndGetTotal(_ context.Context, customerID string, day string) (decimal.Decimal, error) {
	if m.forceError != nil {
		return decimal.Zero, m.forceError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totals[dayKey{customerID, day}], nil
}

func (m *LedgerMemory) UpsertTotal(_ context.Context, customerID string, day string, newTotal decimal.Decimal) error {
	if m.forceError != nil {
		return m.forceError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totals[dayKey{customerID, day}] = newTotal
	return nil
}

func (m *LedgerMemory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totals = map[dayKey]decimal.Decimal{}
}

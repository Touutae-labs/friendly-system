package limit

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var _ DailyLedger = (*Memory)(nil)

type dayKey struct {
	customerID string
	day        string
}

type Memory struct {
	mu     sync.Mutex
	totals map[dayKey]decimal.Decimal
}

func NewMemory() *Memory {
	return &Memory{totals: map[dayKey]decimal.Decimal{}}
}

func (m *Memory) Record(customerID string, qty decimal.Decimal, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := dayKey{customerID, now.UTC().Format(time.DateOnly)}
	m.totals[k] = m.totals[k].Add(qty)
}

func (m *Memory) TradedToday(_ context.Context, customerID string, now time.Time) (decimal.Decimal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := dayKey{customerID, now.UTC().Format(time.DateOnly)}
	return m.totals[k], nil
}

func (m *Memory) LockAndGetTotal(ctx context.Context, _ *gorm.DB, customerID string, day string) (decimal.Decimal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totals[dayKey{customerID, day}], nil
}

func (m *Memory) UpsertTotal(_ context.Context, _ *gorm.DB, customerID string, day string, newTotal decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totals[dayKey{customerID, day}] = newTotal
	return nil
}

func (m *Memory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totals = map[dayKey]decimal.Decimal{}
}

package limit

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
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
	k := dayKey{customerID, now.Format("2006-01-02")}
	m.totals[k] = m.totals[k].Add(qty)
}

func (m *Memory) TradedToday(customerID string, now time.Time) (decimal.Decimal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := dayKey{customerID, now.Format("2006-01-02")}
	return m.totals[k], nil
}

func (m *Memory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totals = map[dayKey]decimal.Decimal{}
}

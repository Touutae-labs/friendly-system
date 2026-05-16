package balance

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var _ AccountRepository = (*Memory)(nil)

type Memory struct {
	mu       sync.RWMutex
	balances map[string]decimal.Decimal
}

func NewMemory(seed map[string]decimal.Decimal) *Memory {
	cloned := make(map[string]decimal.Decimal, len(seed))
	for k, v := range seed {
		cloned[k] = v
	}
	return &Memory{balances: cloned}
}

func (m *Memory) Balance(_ context.Context, customerID string) (decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.balances[customerID]
	if !ok {
		return decimal.Zero, ErrCustomerNotFound
	}
	return b, nil
}

func (m *Memory) LockAndGetBalance(ctx context.Context, _ *gorm.DB, customerID string) (decimal.Decimal, error) {
	return m.Balance(ctx, customerID)
}

func (m *Memory) UpdateBalance(_ context.Context, _ *gorm.DB, customerID string, newBalance decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[customerID] = newBalance
	return nil
}

func (m *Memory) Set(customerID string, bal decimal.Decimal) {
	_ = m.UpdateBalance(context.Background(), nil, customerID, bal)
}

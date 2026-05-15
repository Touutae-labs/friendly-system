package balance

import (
	"sync"

	"github.com/shopspring/decimal"
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

func (m *Memory) Balance(customerID string) (decimal.Decimal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.balances[customerID]
	if !ok {
		return decimal.Zero, ErrCustomerNotFound
	}
	return b, nil
}

func (m *Memory) Set(customerID string, bal decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[customerID] = bal
}

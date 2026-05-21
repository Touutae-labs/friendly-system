package testhelpers

import (
	"context"
	"sync"

	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/shopspring/decimal"
)

var _ repositories.AccountRepository = (*AccountMemory)(nil)

type AccountMemory struct {
	mu         sync.RWMutex
	balances   map[string]decimal.Decimal
	forceError error
}

func NewAccountMemory(seed map[string]decimal.Decimal) *AccountMemory {
	cloned := make(map[string]decimal.Decimal, len(seed))
	for k, v := range seed {
		cloned[k] = v
	}
	return &AccountMemory{balances: cloned}
}

// WithError makes all subsequent calls return err, letting tests simulate
// infrastructure failures without needing generated mocks.
func (m *AccountMemory) WithError(err error) *AccountMemory {
	m.forceError = err
	return m
}

func (m *AccountMemory) Balance(_ context.Context, customerID string) (decimal.Decimal, error) {
	if m.forceError != nil {
		return decimal.Zero, m.forceError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.balances[customerID]
	if !ok {
		return decimal.Zero, repositories.ErrCustomerNotFound
	}
	return b, nil
}

func (m *AccountMemory) LockAndGetBalance(ctx context.Context, customerID string) (decimal.Decimal, error) {
	return m.Balance(ctx, customerID)
}

func (m *AccountMemory) UpdateBalance(_ context.Context, customerID string, newBalance decimal.Decimal) error {
	if m.forceError != nil {
		return m.forceError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[customerID] = newBalance
	return nil
}

func (m *AccountMemory) Set(customerID string, bal decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[customerID] = bal
}

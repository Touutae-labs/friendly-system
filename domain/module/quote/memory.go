package quote

import (
	"context"

	"github.com/shopspring/decimal"
)

var _ MarketPriceProvider = Memory{}

type Memory struct {
	Price decimal.Decimal
}

func NewMemory(price decimal.Decimal) Memory {
	return Memory{Price: price}
}

func (m Memory) CurrentPrice(_ context.Context) (decimal.Decimal, error) {
	if !m.Price.IsPositive() {
		return decimal.Zero, ErrPriceUnavailable
	}
	return m.Price, nil
}

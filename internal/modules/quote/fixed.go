package quote

import (
	"context"

	"github.com/shopspring/decimal"
)

var _ MarketPriceProvider = Fixed{}

// Fixed returns a constant price. Useful for local dev and tests.
type Fixed struct {
	Price decimal.Decimal
}

func NewFixed(price decimal.Decimal) Fixed {
	return Fixed{Price: price}
}

func (f Fixed) CurrentPrice(_ context.Context) (decimal.Decimal, error) {
	if !f.Price.IsPositive() {
		return decimal.Zero, ErrPriceUnavailable
	}
	return f.Price, nil
}

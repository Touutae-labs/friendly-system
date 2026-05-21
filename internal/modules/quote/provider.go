package quote

import (
	"context"
	"errors"

	"github.com/shopspring/decimal"
)

var ErrPriceUnavailable = errors.New("market price unavailable")

// MarketPriceProvider is a read-only gateway to the upstream market price feed.
// Naming follows the Provider pattern (not Repository) because we don't own
// or persist the data — we fetch it from an external source on demand.
type MarketPriceProvider interface {
	CurrentPrice(ctx context.Context) (decimal.Decimal, error)
}

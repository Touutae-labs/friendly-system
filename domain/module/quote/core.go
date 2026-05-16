package quote

import (
	"context"
	"fmt"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
)

const (
	CodeMarketUnavailable = "MARKET_UNAVAILABLE"
	CodeStaleOrOffPrice   = "STALE_OR_OFF_PRICE"
	CodeStalePrice        = "STALE_PRICE"
)

var (
	decOne     = decimal.NewFromInt(1)
	decHundred = decimal.NewFromInt(100)
)

type Config struct {
	PriceTolerance decimal.Decimal
	SpreadMargin   decimal.Decimal
}

func DefaultConfig() Config {
	return Config{
		PriceTolerance: decimal.RequireFromString("0.02"),
		SpreadMargin:   decimal.RequireFromString("0.005"),
	}
}

func (c Config) Validate() error {
	if !c.PriceTolerance.IsPositive() {
		return fmt.Errorf("PriceTolerance must be > 0, got %s", c.PriceTolerance)
	}
	if c.PriceTolerance.GreaterThanOrEqual(decOne) {
		return fmt.Errorf("PriceTolerance must be < 1, got %s", c.PriceTolerance)
	}
	if c.SpreadMargin.IsNegative() {
		return fmt.Errorf("SpreadMargin must be >= 0, got %s", c.SpreadMargin)
	}
	if c.SpreadMargin.GreaterThanOrEqual(decOne) {
		return fmt.Errorf("SpreadMargin must be < 1, got %s", c.SpreadMargin)
	}
	return nil
}

type Validator struct {
	cfg      Config
	provider MarketPriceProvider
}

func New(cfg Config, provider MarketPriceProvider) (*Validator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("quote config: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("quote: market price provider is required")
	}
	return &Validator{cfg: cfg, provider: provider}, nil
}

func (v *Validator) Apply(ctx context.Context, o order.Order, res *order.Result) (marketOK bool) {
	mkt, err := v.provider.CurrentPrice(ctx)
	if err != nil {
		res.AddError(CodeMarketUnavailable, "",
			"current market price is unavailable; cannot validate quote")
		return false
	}

	switch o.OrderType {
	case order.Buy:
		v.checkBuy(o, mkt, res)
	case order.Sell:
		v.checkSell(o, mkt, res)
	}
	return true
}

func (v *Validator) checkBuy(o order.Order, market decimal.Decimal, res *order.Result) {
	expectedBuy := market.Mul(decOne.Add(v.cfg.SpreadMargin))
	spread := expectedBuy.Sub(market)
	res.ExpectedBuyPrice = &expectedBuy
	res.Spread = &spread

	if !withinTolerance(o.QuotedPrice, expectedBuy, v.cfg.PriceTolerance) {
		res.AddError(CodeStaleOrOffPrice, "quoted_price",
			fmt.Sprintf("quoted_price %s is not within %s%% of expected buy price %s",
				o.QuotedPrice.StringFixed(2),
				v.cfg.PriceTolerance.Mul(decHundred).String(),
				expectedBuy.StringFixed(2)))
	}
}

func (v *Validator) checkSell(o order.Order, market decimal.Decimal, res *order.Result) {
	if !withinTolerance(o.QuotedPrice, market, v.cfg.PriceTolerance) {
		res.AddError(CodeStalePrice, "quoted_price",
			fmt.Sprintf("quoted_price %s is not within %s%% of current market price %s",
				o.QuotedPrice.StringFixed(2),
				v.cfg.PriceTolerance.Mul(decHundred).String(),
				market.StringFixed(2)))
	}
}

// withinTolerance reports whether |actual - expected| / expected ≤ tol.
// The boundary case (exactly tol) is *accepted* — this matches our test
// `TestSellWithinToleranceAccepted`. Returns false when expected is zero to
// avoid division-by-zero; callers treat that as out-of-tolerance.
func withinTolerance(actual, expected, tol decimal.Decimal) bool {
	if expected.IsZero() {
		return false
	}
	diff := actual.Sub(expected).Abs()
	pct := diff.Div(expected)
	return pct.LessThanOrEqual(tol)
}

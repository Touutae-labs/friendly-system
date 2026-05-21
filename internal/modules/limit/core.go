package limit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/shopspring/decimal"
)

const (
	CodeLedgerLookupFailed = "LEDGER_LOOKUP_FAILED"
	CodeDailyLimitExceeded = "DAILY_LIMIT_EXCEEDED"
)

type Config struct {
	DailyLimit decimal.Decimal
	// Clock overrides time.Now for testing. Leave nil for production.
	Clock func() time.Time
}

func DefaultConfig() Config {
	return Config{
		DailyLimit: decimal.RequireFromString("5"),
	}
}

func (c Config) Validate() error {
	if !c.DailyLimit.IsPositive() {
		return fmt.Errorf("DailyLimit must be > 0, got %s", c.DailyLimit)
	}
	return nil
}

type Module interface {
	Validate(ctx context.Context, o order.Order, res *order.Result)
	Apply(ctx context.Context, o order.Order) (decimal.Decimal, error)
}

type moduleImpl struct {
	cfg    Config
	ledger repositories.DailyLedger
	now    func() time.Time
}

func New(cfg Config, ledger repositories.DailyLedger) (Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("limit config: %w", err)
	}
	if ledger == nil {
		return nil, fmt.Errorf("limit: daily ledger is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &moduleImpl{cfg: cfg, ledger: ledger, now: clock}, nil
}

func (m *moduleImpl) Validate(ctx context.Context, o order.Order, res *order.Result) {
	traded, err := m.ledger.TradedToday(ctx, o.CustomerID, m.now())
	if err != nil {
		res.AddError(CodeLedgerLookupFailed, "", "could not read daily trading history")
		return
	}
	remaining := decimal.Max(m.cfg.DailyLimit.Sub(traded), decimal.Zero)
	if o.Quantity.GreaterThan(remaining) {
		res.DailyRemaining = &remaining
		res.AddError(CodeDailyLimitExceeded, "quantity",
			fmt.Sprintf("order quantity %s exceeds remaining daily allowance %s baht-weight",
				o.Quantity.String(), remaining.String()))
		return
	}
	afterOrder := remaining.Sub(o.Quantity)
	res.DailyRemaining = &afterOrder
}

func (m *moduleImpl) Apply(ctx context.Context, o order.Order) (decimal.Decimal, error) {
	dayKey := m.now().UTC().Format(time.DateOnly)
	current, err := m.ledger.LockAndGetTotal(ctx, o.CustomerID, dayKey)
	if err != nil {
		return decimal.Zero, err
	}

	next := current.Add(o.Quantity)
	if next.GreaterThan(m.cfg.DailyLimit) {
		return decimal.Max(m.cfg.DailyLimit.Sub(current), decimal.Zero), fmt.Errorf("%s: %w", CodeDailyLimitExceeded, errors.New("daily limit exceeded"))
	}

	if err := m.ledger.UpsertTotal(ctx, o.CustomerID, dayKey, next); err != nil {
		return decimal.Zero, err
	}
	return decimal.Max(m.cfg.DailyLimit.Sub(next), decimal.Zero), nil
}

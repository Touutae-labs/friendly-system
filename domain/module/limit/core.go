package limit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	CodeLedgerLookupFailed = "LEDGER_LOOKUP_FAILED"
	CodeDailyLimitExceeded = "DAILY_LIMIT_EXCEEDED"
)

type Config struct {
	DailyLimit decimal.Decimal
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

type Module struct {
	cfg    Config
	ledger DailyLedger
	now    func() time.Time
}

func New(cfg Config, ledger DailyLedger) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("limit config: %w", err)
	}
	if ledger == nil {
		return nil, fmt.Errorf("limit: daily ledger is required")
	}
	return &Module{cfg: cfg, ledger: ledger, now: time.Now}, nil
}

func (m *Module) WithClock(now func() time.Time) *Module {
	if now != nil {
		m.now = now
	}
	return m
}

// Validate is a soft preflight check: it reads daily_total without a lock and
// rejects orders that obviously exceed the limit.
func (m *Module) Validate(ctx context.Context, o order.Order, res *order.Result) {
	traded, err := m.ledger.TradedToday(ctx, o.CustomerID, m.now())
	if err != nil {
		res.AddError(CodeLedgerLookupFailed, "",
			"could not read daily trading history")
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

func (m *Module) Apply(ctx context.Context, tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	dayKey := m.now().UTC().Format(time.DateOnly)
	current, err := m.ledger.LockAndGetTotal(ctx, tx, o.CustomerID, dayKey)
	if err != nil {
		return decimal.Zero, err
	}

	next := current.Add(o.Quantity)
	if next.GreaterThan(m.cfg.DailyLimit) {
		return decimal.Max(m.cfg.DailyLimit.Sub(current), decimal.Zero), fmt.Errorf("%s: %w", CodeDailyLimitExceeded, errors.New("daily limit exceeded"))
	}

	if err := m.ledger.UpsertTotal(ctx, tx, o.CustomerID, dayKey, next); err != nil {
		return decimal.Zero, err
	}
	return decimal.Max(m.cfg.DailyLimit.Sub(next), decimal.Zero), nil
}

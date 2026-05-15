package limit

import (
	"fmt"
	"time"

	"github.com/pantakan/intergold-validator/domain/common/order"
	"github.com/shopspring/decimal"
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

type Validator struct {
	cfg    Config
	ledger DailyLedger
	now    func() time.Time
}

func New(cfg Config, ledger DailyLedger) (*Validator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("limit config: %w", err)
	}
	if ledger == nil {
		return nil, fmt.Errorf("limit: daily ledger is required")
	}
	return &Validator{cfg: cfg, ledger: ledger, now: time.Now}, nil
}

func (v *Validator) WithClock(now func() time.Time) *Validator {
	if now != nil {
		v.now = now
	}
	return v
}

func (v *Validator) Apply(o order.Order, res *order.Result) {
	traded, err := v.ledger.TradedToday(o.CustomerID, v.now())
	if err != nil {
		res.AddError(CodeLedgerLookupFailed, "",
			"could not read daily trading history")
		return
	}
	remaining := v.cfg.DailyLimit.Sub(traded)
	if o.Quantity.GreaterThan(remaining) {
		if remaining.IsNegative() {
			remaining = decimal.Zero
		}
		res.DailyRemaining = &remaining
		res.AddError(CodeDailyLimitExceeded, "quantity",
			fmt.Sprintf("order quantity %s exceeds remaining daily allowance %s baht-weight",
				o.Quantity.String(), remaining.String()))
		return
	}
	afterOrder := remaining.Sub(o.Quantity)
	res.DailyRemaining = &afterOrder
}

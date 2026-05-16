package balance

import (
	"context"
	"errors"
	"fmt"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	CodeCustomerNotFound    = "CUSTOMER_NOT_FOUND"
	CodeAccountLookupFailed = "ACCOUNT_LOOKUP_FAILED"
	CodeInsufficientBalance = "INSUFFICIENT_BALANCE"
)

type Module struct {
	repo AccountRepository
}

func New(repo AccountRepository) (*Module, error) {
	if repo == nil {
		return nil, fmt.Errorf("balance: account repository is required")
	}
	return &Module{repo: repo}, nil
}

func (m *Module) Validate(ctx context.Context, o order.Order, res *order.Result) {
	bal, err := m.repo.Balance(ctx, o.CustomerID)
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			res.AddError(CodeCustomerNotFound, "customer_id", "customer not found")
		} else {
			res.AddError(CodeAccountLookupFailed, "customer_id", "account lookup failed")
		}
		return
	}
	cost := o.Quantity.Mul(o.QuotedPrice)
	if bal.LessThan(cost) {
		res.AddError(CodeInsufficientBalance, "",
			fmt.Sprintf("balance %s THB is less than required %s THB",
				bal.StringFixed(2), cost.StringFixed(2)))
	}
}

func (m *Module) Apply(ctx context.Context, tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	currentBalance, err := m.repo.LockAndGetBalance(ctx, tx, o.CustomerID)
	if err != nil {
		return decimal.Zero, err
	}

	cost := o.Quantity.Mul(o.QuotedPrice)
	var newBalance decimal.Decimal
	switch o.OrderType {
	case order.Buy:
		if currentBalance.LessThan(cost) {
			return decimal.Zero, fmt.Errorf("%s: %w", CodeInsufficientBalance, errors.New("insufficient balance"))
		}
		newBalance = currentBalance.Sub(cost)
	case order.Sell:
		newBalance = currentBalance.Add(cost)
	default:
		return decimal.Zero, fmt.Errorf("unexpected order_type %q", o.OrderType)
	}

	if err := m.repo.UpdateBalance(ctx, tx, o.CustomerID, newBalance); err != nil {
		return decimal.Zero, err
	}
	return newBalance, nil
}

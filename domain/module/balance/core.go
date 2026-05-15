package balance

import (
	"errors"
	"fmt"

	"github.com/pantakan/intergold-validator/domain/common/order"
)

const (
	CodeCustomerNotFound    = "CUSTOMER_NOT_FOUND"
	CodeAccountLookupFailed = "ACCOUNT_LOOKUP_FAILED"
	CodeInsufficientBalance = "INSUFFICIENT_BALANCE"
)

type Validator struct {
	repo AccountRepository
}

func New(repo AccountRepository) (*Validator, error) {
	if repo == nil {
		return nil, fmt.Errorf("balance: account repository is required")
	}
	return &Validator{repo: repo}, nil
}

func (v *Validator) Apply(o order.Order, res *order.Result) {
	bal, err := v.repo.Balance(o.CustomerID)
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

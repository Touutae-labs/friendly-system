package balance

import (
	"errors"

	"github.com/shopspring/decimal"
)

var ErrCustomerNotFound = errors.New("customer not found")

type AccountRepository interface {
	Balance(customerID string) (decimal.Decimal, error)
}

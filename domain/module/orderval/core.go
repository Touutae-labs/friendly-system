package orderval

import (
	"fmt"
	"unicode"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
)

const (
	CodeInvalidOrderType    = "INVALID_ORDER_TYPE"
	CodeInvalidQuantity     = "INVALID_QUANTITY"
	CodeQuantityTooLarge    = "QUANTITY_TOO_LARGE"
	CodeInvalidQuantityIncr = "INVALID_QUANTITY_INCREMENT"
	CodeInvalidPrice        = "INVALID_PRICE"
	CodePriceTooLarge       = "PRICE_TOO_LARGE"
	CodeMissingCustomerID   = "MISSING_CUSTOMER_ID"
	CodeInvalidCustomerID   = "INVALID_CUSTOMER_ID"
)

type Config struct {
	QuantityIncrement   decimal.Decimal
	MaxQuantity         decimal.Decimal
	MaxPrice            decimal.Decimal
	MaxCustomerIDLength int
}

func DefaultConfig() Config {
	return Config{
		QuantityIncrement:   decimal.RequireFromString("0.5"),
		MaxQuantity:         decimal.RequireFromString("1000"),
		MaxPrice:            decimal.RequireFromString("10000000"),
		MaxCustomerIDLength: 64,
	}
}

func (c Config) Validate() error {
	if !c.QuantityIncrement.IsPositive() {
		return fmt.Errorf("QuantityIncrement must be > 0, got %s", c.QuantityIncrement)
	}
	if !c.MaxQuantity.IsPositive() {
		return fmt.Errorf("MaxQuantity must be > 0, got %s", c.MaxQuantity)
	}
	if !c.MaxPrice.IsPositive() {
		return fmt.Errorf("MaxPrice must be > 0, got %s", c.MaxPrice)
	}
	if c.MaxCustomerIDLength <= 0 {
		return fmt.Errorf("MaxCustomerIDLength must be > 0, got %d", c.MaxCustomerIDLength)
	}
	return nil
}

type Validator struct {
	cfg Config
}

func New(cfg Config) (*Validator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("orderval config: %w", err)
	}
	return &Validator{cfg: cfg}, nil
}

func (v *Validator) Apply(o order.Order, res *order.Result) {
	v.checkOrderType(o, res)
	v.checkQuantity(o, res)
	v.checkPrice(o, res)
	v.checkCustomerID(o, res)
}

func (v *Validator) checkOrderType(o order.Order, res *order.Result) {
	switch o.OrderType {
	case order.Buy, order.Sell:
	default:
		res.AddError(CodeInvalidOrderType, "order_type",
			fmt.Sprintf("order_type must be \"buy\" or \"sell\", got %q", string(o.OrderType)))
	}
}

func (v *Validator) checkQuantity(o order.Order, res *order.Result) {
	if !o.Quantity.IsPositive() {
		res.AddError(CodeInvalidQuantity, "quantity",
			fmt.Sprintf("quantity must be positive, got %s", o.Quantity.String()))
		return
	}
	if o.Quantity.GreaterThan(v.cfg.MaxQuantity) {
		res.AddError(CodeQuantityTooLarge, "quantity",
			fmt.Sprintf("quantity %s exceeds maximum %s baht-weight per order",
				o.Quantity.String(), v.cfg.MaxQuantity.String()))
		return
	}
	if !isMultipleOf(o.Quantity, v.cfg.QuantityIncrement) {
		res.AddError(CodeInvalidQuantityIncr, "quantity",
			fmt.Sprintf("quantity %s must be a multiple of %s baht-weight",
				o.Quantity.String(), v.cfg.QuantityIncrement.String()))
	}
}

func (v *Validator) checkPrice(o order.Order, res *order.Result) {
	if !o.QuotedPrice.IsPositive() {
		res.AddError(CodeInvalidPrice, "quoted_price",
			fmt.Sprintf("quoted_price must be positive, got %s", o.QuotedPrice.String()))
		return
	}
	if o.QuotedPrice.GreaterThan(v.cfg.MaxPrice) {
		res.AddError(CodePriceTooLarge, "quoted_price",
			fmt.Sprintf("quoted_price %s exceeds maximum %s THB / baht-weight",
				o.QuotedPrice.String(), v.cfg.MaxPrice.String()))
	}
}

func (v *Validator) checkCustomerID(o order.Order, res *order.Result) {
	id := o.CustomerID
	if id == "" {
		res.AddError(CodeMissingCustomerID, "customer_id", "customer_id is required")
		return
	}
	if len(id) > v.cfg.MaxCustomerIDLength {
		res.AddError(CodeInvalidCustomerID, "customer_id",
			fmt.Sprintf("customer_id must be at most %d bytes, got %d",
				v.cfg.MaxCustomerIDLength, len(id)))
		return
	}
	if id[0] == ' ' || id[len(id)-1] == ' ' {
		res.AddError(CodeInvalidCustomerID, "customer_id",
			"customer_id must not have leading or trailing whitespace")
		return
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			res.AddError(CodeInvalidCustomerID, "customer_id",
				"customer_id contains control characters")
			return
		}
	}
}

func isMultipleOf(value, increment decimal.Decimal) bool {
	if !increment.IsPositive() {
		return false
	}
	q := value.Div(increment)
	return q.Equal(q.Truncate(0))
}

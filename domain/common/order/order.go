package order

import "github.com/shopspring/decimal"

type Type string

const (
	Buy  Type = "buy"
	Sell Type = "sell"
)

type Order struct {
	CustomerID  string          `json:"customer_id"`
	OrderType   Type            `json:"order_type"`
	Quantity    decimal.Decimal `json:"quantity"`
	QuotedPrice decimal.Decimal `json:"quoted_price"`
}

type Result struct {
	Valid            bool             `json:"valid"`
	Errors           []Error          `json:"errors,omitempty"`
	ExpectedBuyPrice *decimal.Decimal `json:"expected_buy_price,omitempty"`
	Spread           *decimal.Decimal `json:"spread,omitempty"`
	DailyRemaining   *decimal.Decimal `json:"daily_remaining,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (r *Result) AddError(code, field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, Error{Code: code, Field: field, Message: message})
}

func (r Result) HasError(code string) bool {
	for _, e := range r.Errors {
		if e.Code == code {
			return true
		}
	}
	return false
}

package orderval

import (
	"testing"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestIsMultipleOf(t *testing.T) {
	cases := []struct {
		v, i string
		want bool
	}{
		{"0.5", "0.5", true},
		{"1", "0.5", true},
		{"2.5", "0.5", true},
		{"0.7", "0.5", false},
		{"0.51", "0.5", false},
		{"0", "0.5", true},
	}
	for _, c := range cases {
		got := isMultipleOf(d(c.v), d(c.i))
		if got != c.want {
			t.Errorf("isMultipleOf(%s, %s) = %v, want %v", c.v, c.i, got, c.want)
		}
	}
}

func TestConfigValidate_RejectsBadValues(t *testing.T) {
	base := DefaultConfig()
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"QuantityIncrement=0", func(c *Config) { c.QuantityIncrement = d("0") }},
		{"MaxQuantity=0", func(c *Config) { c.MaxQuantity = d("0") }},
		{"MaxPrice=neg", func(c *Config) { c.MaxPrice = d("-1") }},
		{"MaxCustomerIDLength=0", func(c *Config) { c.MaxCustomerIDLength = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			if err := c.Validate(); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestValidator_Apply(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCustomerIDLength = 10
	v, _ := New(cfg)

	tests := []struct {
		name      string
		order     order.Order
		wantValid bool
		wantCode  string
	}{
		{
			name: "valid",
			order: order.Order{
				CustomerID:  "C001",
				OrderType:   order.Buy,
				Quantity:    d("1"),
				QuotedPrice: d("100"),
			},
			wantValid: true,
		},
		{
			name: "missing customer id",
			order: order.Order{
				CustomerID:  "",
				OrderType:   order.Buy,
				Quantity:    d("1"),
				QuotedPrice: d("100"),
			},
			wantValid: false,
			wantCode:  CodeMissingCustomerID,
		},
		{
			name: "invalid customer id (too long)",
			order: order.Order{
				CustomerID:  "C0010000000000000000000000000000000000000000",
				OrderType:   order.Buy,
				Quantity:    d("1"),
				QuotedPrice: d("100"),
			},
			wantValid: false,
			wantCode:  CodeInvalidCustomerID,
		},
		{
			name: "invalid quantity (negative)",
			order: order.Order{
				CustomerID:  "C001",
				OrderType:   order.Buy,
				Quantity:    d("-1"),
				QuotedPrice: d("100"),
			},
			wantValid: false,
			wantCode:  CodeInvalidQuantity,
		},
		{
			name: "invalid quantity (not multiple)",
			order: order.Order{
				CustomerID:  "C001",
				OrderType:   order.Buy,
				Quantity:    d("0.7"),
				QuotedPrice: d("100"),
			},
			wantValid: false,
			wantCode:  CodeInvalidQuantityIncr,
		},
		{
			name: "invalid price (zero)",
			order: order.Order{
				CustomerID:  "C001",
				OrderType:   order.Buy,
				Quantity:    d("1"),
				QuotedPrice: d("0"),
			},
			wantValid: false,
			wantCode:  CodeInvalidPrice,
		},
		{
			name: "invalid order type",
			order: order.Order{
				CustomerID:  "C001",
				OrderType:   order.Type("INVALID"),
				Quantity:    d("1"),
				QuotedPrice: d("100"),
			},
			wantValid: false,
			wantCode:  CodeInvalidOrderType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &order.Result{Valid: true}
			v.Apply(tt.order, res)
			if res.Valid != tt.wantValid {
				t.Errorf("expected valid=%v, got %v", tt.wantValid, res.Valid)
			}
			if tt.wantCode != "" && !res.HasError(tt.wantCode) {
				t.Errorf("expected error code %s, got %+v", tt.wantCode, res.Errors)
			}
		})
	}
}

package orderval

import (
	"testing"

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

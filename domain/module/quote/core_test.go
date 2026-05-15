package quote

import (
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestWithinTolerance(t *testing.T) {
	tol := d("0.02")
	if !withinTolerance(d("100"), d("100"), tol) {
		t.Error("equal should be within tolerance")
	}
	if !withinTolerance(d("102"), d("100"), tol) {
		t.Error("2% above should be within tolerance (boundary)")
	}
	if withinTolerance(d("103"), d("100"), tol) {
		t.Error("3% above should not be within tolerance")
	}
	if withinTolerance(d("100"), d("0"), tol) {
		t.Error("expected=0 should never be within tolerance")
	}
}

func TestConfigValidate_RejectsBadValues(t *testing.T) {
	base := DefaultConfig()
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"PriceTolerance=0", func(c *Config) { c.PriceTolerance = d("0") }},
		{"PriceTolerance=1", func(c *Config) { c.PriceTolerance = d("1") }},
		{"SpreadMargin=neg", func(c *Config) { c.SpreadMargin = d("-0.01") }},
		{"SpreadMargin=1", func(c *Config) { c.SpreadMargin = d("1") }},
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

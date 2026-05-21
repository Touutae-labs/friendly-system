package quote

import (
	"context"
	"testing"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
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

func TestValidator_Apply(t *testing.T) {
	mkt := NewFixed(d("100"))
	v, _ := New(DefaultConfig(), mkt)
	ctx := context.Background()

	t.Run("valid buy within tolerance", func(t *testing.T) {
		res := &order.Result{Valid: true}
		// expected buy price = 100 * (1 + 0.005) = 100.5
		o := order.Order{OrderType: order.Buy, QuotedPrice: d("100.5")}
		ok := v.Apply(ctx, o, res)
		if !ok || !res.Valid {
			t.Errorf("expected valid, got %+v", res.Errors)
		}
	})

	t.Run("stale or off price buy", func(t *testing.T) {
		res := &order.Result{Valid: true}
		o := order.Order{OrderType: order.Buy, QuotedPrice: d("110")}
		v.Apply(ctx, o, res)
		if res.Valid || !res.HasError(CodeStaleOrOffPrice) {
			t.Errorf("expected error %s, got %+v", CodeStaleOrOffPrice, res.Errors)
		}
	})

	t.Run("valid sell within tolerance", func(t *testing.T) {
		res := &order.Result{Valid: true}
		o := order.Order{OrderType: order.Sell, QuotedPrice: d("99")} // tolerance is 2%
		v.Apply(ctx, o, res)
		if !res.Valid {
			t.Errorf("expected valid, got %+v", res.Errors)
		}
	})

	t.Run("stale price sell", func(t *testing.T) {
		res := &order.Result{Valid: true}
		o := order.Order{OrderType: order.Sell, QuotedPrice: d("95")}
		v.Apply(ctx, o, res)
		if res.Valid || !res.HasError(CodeStalePrice) {
			t.Errorf("expected error %s, got %+v", CodeStalePrice, res.Errors)
		}
	})

	t.Run("market unavailable", func(t *testing.T) {
		mkt2 := NewFixed(d("0"))
		v2, _ := New(DefaultConfig(), mkt2)
		res := &order.Result{Valid: true}
		o := order.Order{OrderType: order.Buy, QuotedPrice: d("100")}
		ok := v2.Apply(ctx, o, res)
		if ok || res.Valid || !res.HasError(CodeMarketUnavailable) {
			t.Errorf("expected error %s, got %+v", CodeMarketUnavailable, res.Errors)
		}
	})
}

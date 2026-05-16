package balance

import (
	"context"
	"testing"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestModule_Validate(t *testing.T) {
	repo := NewMemory(map[string]decimal.Decimal{
		"C001": d("1000"),
	})
	m, _ := New(repo)

	tests := []struct {
		name       string
		customerID string
		qty        string
		price      string
		wantError  string
	}{
		{
			name:       "valid buy",
			customerID: "C001",
			qty:        "1",
			price:      "1000",
			wantError:  "",
		},
		{
			name:       "insufficient balance",
			customerID: "C001",
			qty:        "1.1",
			price:      "1000",
			wantError:  CodeInsufficientBalance,
		},
		{
			name:       "customer not found",
			customerID: "GHOST",
			qty:        "1",
			price:      "100",
			wantError:  CodeCustomerNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := &order.Result{Valid: true}
			m.Validate(context.Background(), order.Order{
				CustomerID:  tt.customerID,
				Quantity:    d(tt.qty),
				QuotedPrice: d(tt.price),
				OrderType:   order.Buy,
			}, res)

			if tt.wantError == "" {
				if !res.Valid {
					t.Errorf("expected valid, got errors: %+v", res.Errors)
				}
			} else {
				if !res.HasError(tt.wantError) {
					t.Errorf("expected error %s, got %+v", tt.wantError, res.Errors)
				}
			}
		})
	}
}

func TestModule_Apply(t *testing.T) {
	repo := NewMemory(map[string]decimal.Decimal{
		"C001": d("1000"),
	})
	m, _ := New(repo)

	t.Run("buy success", func(t *testing.T) {
		bal, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID:  "C001",
			Quantity:    d("0.5"),
			QuotedPrice: d("1000"),
			OrderType:   order.Buy,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bal.Equal(d("500")) {
			t.Errorf("expected balance 500, got %s", bal)
		}
	})

	t.Run("sell success", func(t *testing.T) {
		bal, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID:  "C001",
			Quantity:    d("1"),
			QuotedPrice: d("1000"),
			OrderType:   order.Sell,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bal.Equal(d("1500")) {
			t.Errorf("expected balance 1500, got %s", bal)
		}
	})

	t.Run("insufficient balance", func(t *testing.T) {
		repo.Set("C001", d("100"))
		_, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID:  "C001",
			Quantity:    d("1"),
			QuotedPrice: d("200"),
			OrderType:   order.Buy,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("customer not found", func(t *testing.T) {
		_, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID:  "GHOST",
			Quantity:    d("1"),
			QuotedPrice: d("100"),
			OrderType:   order.Buy,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

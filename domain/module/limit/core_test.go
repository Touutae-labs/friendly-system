package limit

import (
	"context"
	"testing"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestModule_Validate(t *testing.T) {
	led := NewMemory()
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	m, _ := New(DefaultConfig(), led)
	m = m.WithClock(func() time.Time { return now })

	tests := []struct {
		name       string
		customerID string
		qty        string
		traded     string
		wantError  string
	}{
		{
			name:       "under limit",
			customerID: "C001",
			qty:        "1",
			traded:     "3",
			wantError:  "",
		},
		{
			name:       "exact limit",
			customerID: "C001",
			qty:        "2",
			traded:     "3",
			wantError:  "",
		},
		{
			name:       "over limit",
			customerID: "C001",
			qty:        "2.1",
			traded:     "3",
			wantError:  CodeDailyLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			led.Reset()
			if tt.traded != "" {
				led.Record(tt.customerID, d(tt.traded), now)
			}

			res := &order.Result{Valid: true}
			m.Validate(context.Background(), order.Order{
				CustomerID: tt.customerID,
				Quantity:   d(tt.qty),
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
	led := NewMemory()
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	m, _ := New(DefaultConfig(), led)
	m = m.WithClock(func() time.Time { return now })

	t.Run("apply success", func(t *testing.T) {
		led.Reset()
		remaining, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID: "C001",
			Quantity:   d("2"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !remaining.Equal(d("3")) {
			t.Errorf("expected remaining 3, got %s", remaining)
		}

		// check record
		traded, _ := led.TradedToday(context.Background(), "C001", now)
		if !traded.Equal(d("2")) {
			t.Errorf("expected traded 2, got %s", traded)
		}
	})

	t.Run("apply exceeded", func(t *testing.T) {
		led.Reset()
		led.Record("C001", d("4"), now)
		_, err := m.Apply(context.Background(), nil, order.Order{
			CustomerID: "C001",
			Quantity:   d("1.1"),
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

package testhelpers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestLedgerMemory_ConcurrentRecord(t *testing.T) {
	led := NewLedgerMemory()
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	const goroutines = 50
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				led.Record("C001", decimal.RequireFromString("0.1"), now)
			}
		}()
	}
	wg.Wait()
	got, err := led.TradedToday(context.Background(), "C001", now)
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	want := decimal.RequireFromString("0.1").Mul(decimal.NewFromInt(goroutines * perGoroutine))
	if !got.Equal(want) {
		t.Errorf("expected total %s, got %s (race / lost updates?)", want, got)
	}
}

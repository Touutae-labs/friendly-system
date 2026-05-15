package repositories_test

import (
	"testing"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/repositories"
	"github.com/shopspring/decimal"
)

func TestLedger_TradedToday_Empty(t *testing.T) {
	repo := repositories.NewLedger(newDB(t))
	got, err := repo.TradedToday("C001", time.Now())
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("empty ledger should return 0, got %s", got)
	}
}

func TestLedger_RecordThenRead(t *testing.T) {
	repo := repositories.NewLedger(newDB(t))
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	if err := repo.Record("C001", decimal.RequireFromString("1.5"), now); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := repo.Record("C001", decimal.RequireFromString("2"), now); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := repo.TradedToday("C001", now)
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("3.5")) {
		t.Errorf("got %s, want 3.5", got)
	}
}

func TestLedger_DayBoundary(t *testing.T) {
	repo := repositories.NewLedger(newDB(t))
	yesterday := time.Date(2026, 5, 14, 23, 0, 0, 0, time.UTC)
	today := time.Date(2026, 5, 15, 1, 0, 0, 0, time.UTC)

	if err := repo.Record("C001", decimal.RequireFromString("3"), yesterday); err != nil {
		t.Fatalf("Record yesterday: %v", err)
	}
	if err := repo.Record("C001", decimal.RequireFromString("1"), today); err != nil {
		t.Fatalf("Record today: %v", err)
	}

	got, err := repo.TradedToday("C001", today)
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("1")) {
		t.Errorf("today should be 1, got %s (yesterday must not leak)", got)
	}
}

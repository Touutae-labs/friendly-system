package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/shopspring/decimal"
)

func TestLedger_TradedToday_Empty(t *testing.T) {
	repo := repositories.NewLedger(newDB(t))
	got, err := repo.TradedToday(context.Background(), "C001", time.Now())
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("empty ledger should return 0, got %s", got)
	}
}

func TestLedger_TradedToday_ExistingRecord(t *testing.T) {
	db := newDB(t)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	day := now.UTC().Format(time.DateOnly)

	if err := db.Create(&models.DailyTotalModel{
		CustomerID: "C001",
		Day:        day,
		Total:      decimal.RequireFromString("3.5"),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	repo := repositories.NewLedger(db)
	got, err := repo.TradedToday(context.Background(), "C001", now)
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("3.5")) {
		t.Errorf("got %s, want 3.5", got)
	}
}

func TestLedger_DayBoundary(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	yesterday := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)

	if err := db.Create(&models.DailyTotalModel{
		CustomerID: "C001",
		Day:        yesterday.Format(time.DateOnly),
		Total:      decimal.RequireFromString("3"),
	}).Error; err != nil {
		t.Fatalf("seed yesterday: %v", err)
	}
	if err := db.Create(&models.DailyTotalModel{
		CustomerID: "C001",
		Day:        today.Format(time.DateOnly),
		Total:      decimal.RequireFromString("1"),
	}).Error; err != nil {
		t.Fatalf("seed today: %v", err)
	}

	repo := repositories.NewLedger(db)
	got, err := repo.TradedToday(ctx, "C001", today)
	if err != nil {
		t.Fatalf("TradedToday: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("1")) {
		t.Errorf("today should be 1, got %s (yesterday must not leak)", got)
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
	"gorm.io/gorm/clause"

	"github.com/Touutae-labs/friendly-system/internal/configurations"
	"github.com/Touutae-labs/friendly-system/internal/di"
	"github.com/Touutae-labs/friendly-system/internal/models"
)

func newTestApp(t *testing.T) *fiber.App {
	t.Helper()
	cfg := configurations.Config{
		Server: configurations.ServerConfig{
			Port:    "0",
			Title:   "friendly-system-test",
			Version: "test",
		},
	}
	app, cleanup, err := di.InitializeApp(cfg, "friendly-system-test", "test")
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	t.Cleanup(cleanup)
	seedTestData(t, app)
	return app.Server.App
}

func seedTestData(t *testing.T, app *di.Application) {
	t.Helper()
	db := app.DB
	accounts := []models.AccountModel{
		{CustomerID: "C001", Balance: decimal.RequireFromString("1000000")},
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"balance"}),
	}).Create(&accounts).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	dailyTotal := models.DailyTotalModel{
		CustomerID: "C001",
		Day:        today,
		Total:      decimal.RequireFromString("4"),
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"total"}),
	}).Create(&dailyTotal).Error; err != nil {
		t.Fatalf("seed daily total: %v", err)
	}
}

func TestHealth(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
}

func TestValidate_HappyBuy(t *testing.T) {
	app := newTestApp(t)
	body := strings.NewReader(`{
		"customer_id": "C001",
		"order_type": "buy",
		"quantity": "0.5",
		"quoted_price": "42210"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if valid, _ := out["valid"].(bool); !valid {
		t.Errorf("expected valid=true, got body=%v", out)
	}
	if _, hasSpread := out["spread"]; !hasSpread {
		t.Errorf("expected spread field for buy, got body=%v", out)
	}
}

func TestValidate_DailyLimitExceeded(t *testing.T) {
	app := newTestApp(t)
	body := strings.NewReader(`{
		"customer_id": "C001",
		"order_type": "buy",
		"quantity": "1.5",
		"quoted_price": "42210"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	errs, _ := out["errors"].([]any)
	found := false
	for _, e := range errs {
		if em, ok := e.(map[string]any); ok {
			if em["code"] == "DAILY_LIMIT_EXCEEDED" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected DAILY_LIMIT_EXCEEDED in response, got %v", out)
	}
}

func TestValidate_BadJSON(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

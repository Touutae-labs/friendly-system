package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pantakan/intergold-validator/wire"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	svc, err := wire.InitValidatorService()
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /orders/validate", handleValidate(svc, logger))
	// in-memory mode skips /orders/process — tests for that endpoint
	// live in domain/service/processor (against an in-memory GORM DB)
	return loggingMiddleware(logger, mux)
}

func TestHealth(t *testing.T) {
	mux := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
}

func TestValidate_HappyBuy(t *testing.T) {
	mux := newTestServer(t)
	body := strings.NewReader(`{
		"customer_id": "C001",
		"order_type": "buy",
		"quantity": "0.5",
		"quoted_price": "42210"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if valid, _ := out["valid"].(bool); !valid {
		t.Errorf("expected valid=true, got body=%s", rec.Body.String())
	}
	if _, hasSpread := out["spread"]; !hasSpread {
		t.Errorf("expected spread field present for buy, got body=%s", rec.Body.String())
	}
}

func TestValidate_DailyLimitExceeded(t *testing.T) {
	mux := newTestServer(t)
	body := strings.NewReader(`{
		"customer_id": "C001",
		"order_type": "buy",
		"quantity": "1.5",
		"quoted_price": "42210"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "DAILY_LIMIT_EXCEEDED") {
		t.Errorf("expected DAILY_LIMIT_EXCEEDED in response, got %s", rec.Body.String())
	}
}

func TestValidate_BadJSON(t *testing.T) {
	mux := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", bytes.NewBufferString(`{not json`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", rec.Code)
	}
}

func TestValidate_UnknownField(t *testing.T) {
	mux := newTestServer(t)
	body := strings.NewReader(`{
		"customer_id": "C001",
		"order_type": "buy",
		"quantity": "1",
		"quoted_price": "42210",
		"extra_field": "nope"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/orders/validate", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400 (unknown field should reject)", rec.Code)
	}
}

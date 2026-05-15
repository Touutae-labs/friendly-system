package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/pantakan/intergold-validator/domain/common/order"
	"github.com/pantakan/intergold-validator/domain/service/processor"
	validatorsvc "github.com/pantakan/intergold-validator/domain/service/validator"
	"github.com/pantakan/intergold-validator/wire"
	"gorm.io/gorm"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "", "SQLite database path (omit for in-memory adapters)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	svc, proc, cleanup, err := buildService(*dbPath)
	if err != nil {
		logger.Error("build service", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /orders/validate", handleValidate(svc, logger))
	if proc != nil {
		mux.HandleFunc("POST /orders/process", handleProcess(proc, logger))
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           loggingMiddleware(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, syscall.SIGINT, syscall.SIGTERM)
		<-sigint
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown", "err", err)
		}
		close(idleConnsClosed)
	}()

	mode := "in-memory"
	if *dbPath != "" {
		mode = "gorm-sqlite:" + *dbPath
	}
	logger.Info("server starting", "addr", *addr, "mode", mode)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("listen", "err", err)
		os.Exit(1)
	}
	<-idleConnsClosed
	logger.Info("server stopped")
}

// loggingMiddleware emits one structured log line per HTTP request with
// method, path, status, and duration. Captures status via a wrapped
// ResponseWriter.
func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleValidate(svc *validatorsvc.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var o order.Order
		if err := dec.Decode(&o); err != nil {
			logger.Warn("decode order", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}

		result := svc.Validate(o)

		attrs := []any{
			"customer_id", o.CustomerID,
			"order_type", string(o.OrderType),
			"quantity", o.Quantity.String(),
			"quoted_price", o.QuotedPrice.String(),
			"valid", result.Valid,
		}
		if !result.Valid && len(result.Errors) > 0 {
			codes := make([]string, len(result.Errors))
			for i, e := range result.Errors {
				codes[i] = e.Code
			}
			attrs = append(attrs, "error_codes", codes)
		}
		logger.Info("validate", attrs...)

		writeJSON(w, http.StatusOK, result)
	}
}

func handleProcess(proc *processor.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Idempotency-Key header is required",
			})
			return
		}

		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var o order.Order
		if err := dec.Decode(&o); err != nil {
			logger.Warn("decode order", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}

		result := proc.Process(key, o)

		logger.Info("process",
			"customer_id", o.CustomerID,
			"order_type", string(o.OrderType),
			"quantity", o.Quantity.String(),
			"status", string(result.Status),
			"order_id", result.OrderID,
			"idempotency_key", key,
		)

		switch result.Status {
		case processor.StatusFilled, processor.StatusDuplicate:
			writeJSON(w, http.StatusOK, result)
		case processor.StatusRejected:
			writeJSON(w, http.StatusUnprocessableEntity, result)
		default:
			writeJSON(w, http.StatusInternalServerError, result)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func buildService(dbPath string) (*validatorsvc.Service, *processor.Service, func(), error) {
	if dbPath == "" {
		// In-memory mode has no transactional store, so /orders/process is
		// not exposed. /orders/validate still works.
		svc, err := wire.InitValidatorService()
		return svc, nil, func() {}, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	svc, err := wire.InitValidatorServiceGorm(db)
	if err != nil {
		return nil, nil, nil, err
	}
	proc, err := wire.InitProcessorService(db)
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup := func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}
	return svc, proc, cleanup, nil
}

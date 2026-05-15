package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/pantakan/intergold-validator/domain/common/order"
	validatorsvc "github.com/pantakan/intergold-validator/domain/service/validator"
	"github.com/pantakan/intergold-validator/wire"
	"gorm.io/gorm"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "", "SQLite database path (omit for in-memory adapters)")
	flag.Parse()

	svc, cleanup, err := buildService(*dbPath)
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /orders/validate", handleValidate(svc))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
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
			log.Printf("shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	mode := "in-memory adapters"
	if *dbPath != "" {
		mode = fmt.Sprintf("GORM/SQLite db=%s", *dbPath)
	}
	log.Printf("listening on %s (%s)", *addr, mode)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	<-idleConnsClosed
	log.Println("shutdown complete")
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleValidate(svc *validatorsvc.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		var o order.Order
		if err := dec.Decode(&o); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid request body: %v", err),
			})
			return
		}
		writeJSON(w, http.StatusOK, svc.Validate(o))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func buildService(dbPath string) (*validatorsvc.Service, func(), error) {
	if dbPath == "" {
		svc, err := wire.InitValidatorService()
		return svc, func() {}, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	svc, err := wire.InitValidatorServiceGorm(db)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}
	return svc, cleanup, nil
}

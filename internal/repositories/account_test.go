package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAccount_Balance_HappyPath(t *testing.T) {
	db := newDB(t)
	if err := db.Create(&models.AccountModel{
		CustomerID: "C001",
		Balance:    decimal.RequireFromString("1000000.50"),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repositories.NewAccount(db)
	got, err := repo.Balance(context.Background(), "C001")
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if !got.Equal(decimal.RequireFromString("1000000.50")) {
		t.Errorf("got %s, want 1000000.50", got)
	}
}

func TestAccount_Balance_NotFound(t *testing.T) {
	db := newDB(t)
	repo := repositories.NewAccount(db)
	_, err := repo.Balance(context.Background(), "GHOST")
	if !errors.Is(err, repositories.ErrCustomerNotFound) {
		t.Errorf("expected ErrCustomerNotFound, got %v", err)
	}
}

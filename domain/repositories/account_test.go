package repositories_test

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/pantakan/intergold-validator/domain/module/balance"
	"github.com/pantakan/intergold-validator/domain/repositories"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(repositories.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAccount_Balance_HappyPath(t *testing.T) {
	db := newDB(t)
	if err := db.Create(&repositories.AccountModel{CustomerID: "C001", Balance: "1000000.50"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repositories.NewAccount(db)
	got, err := repo.Balance("C001")
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
	_, err := repo.Balance("GHOST")
	if !errors.Is(err, balance.ErrCustomerNotFound) {
		t.Errorf("expected ErrCustomerNotFound, got %v", err)
	}
}

func TestAccount_Balance_MalformedData(t *testing.T) {
	db := newDB(t)
	if err := db.Create(&repositories.AccountModel{CustomerID: "C001", Balance: "not-a-number"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repositories.NewAccount(db)
	_, err := repo.Balance("C001")
	if err == nil {
		t.Error("expected parse error, got nil")
	}
	if errors.Is(err, balance.ErrCustomerNotFound) {
		t.Errorf("malformed data should not surface as ErrCustomerNotFound, got %v", err)
	}
}

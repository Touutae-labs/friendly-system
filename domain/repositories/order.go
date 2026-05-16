package repositories

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type OrderRepository interface {
	FindPrior(ctx context.Context, tx *gorm.DB, idempotencyKey string) (*OrderModel, error)
	Create(ctx context.Context, tx *gorm.DB, row *OrderModel) error
}

type orderRepo struct {
	db *gorm.DB
}

func NewOrder(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

func (r *orderRepo) FindPrior(ctx context.Context, tx *gorm.DB, key string) (*OrderModel, error) {
	db := r.db
	if tx != nil {
		db = tx
	}
	var row OrderModel
	err := db.WithContext(ctx).Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repositories.Order: find: %w", err)
	}
	return &row, nil
}

func (r *orderRepo) Create(ctx context.Context, tx *gorm.DB, row *OrderModel) error {
	db := r.db
	if tx != nil {
		db = tx
	}
	return db.WithContext(ctx).Create(row).Error
}

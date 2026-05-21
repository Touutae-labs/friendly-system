package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/txutil"
	"gorm.io/gorm"
)

type OrderRepository interface {
	FindPrior(ctx context.Context, idempotencyKey string) (*models.OrderModel, error)
	Create(ctx context.Context, row *models.OrderModel) error
}

type orderRepo struct {
	db *gorm.DB
}

func NewOrder(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

func (r *orderRepo) FindPrior(ctx context.Context, key string) (*models.OrderModel, error) {
	db := txutil.FromContext(ctx, r.db)
	var row models.OrderModel
	err := db.WithContext(ctx).Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repositories.Order: find: %w", err)
	}
	return &row, nil
}

func (r *orderRepo) Create(ctx context.Context, row *models.OrderModel) error {
	db := txutil.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(row).Error
}

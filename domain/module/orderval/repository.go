package orderval

import (
	"context"

	"github.com/Touutae-labs/friendly-system/domain/repositories"
	"gorm.io/gorm"
)

type OrderRepository interface {
	FindPrior(ctx context.Context, tx *gorm.DB, idempotencyKey string) (*repositories.OrderModel, error)
	Create(ctx context.Context, tx *gorm.DB, row *repositories.OrderModel) error
}

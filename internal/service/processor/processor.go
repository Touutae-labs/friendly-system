package processor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/modules/balance"
	"github.com/Touutae-labs/friendly-system/internal/modules/limit"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/Touutae-labs/friendly-system/internal/txutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type ProcessorService interface {
	Process(ctx context.Context, idempotencyKey string, o order.Order) order.ProcessResult
}

// validatorService is a local interface for the validator dependency,
// avoiding a direct import of the validatorsvc package.
type validatorService interface {
	Validate(ctx context.Context, o order.Order) order.Result
}

type processorServiceImpl struct {
	db         *gorm.DB
	validator  validatorService
	balance    balance.Module
	limit      limit.Module
	orders     repositories.OrderRepository
	dailyLimit decimal.Decimal
	now        func() time.Time
	newID      func() string
}

func New(
	db *gorm.DB,
	validator validatorService,
	b balance.Module,
	l limit.Module,
	orders repositories.OrderRepository,
	limitCfg limit.Config,
) ProcessorService {
	return &processorServiceImpl{
		db:         db,
		validator:  validator,
		balance:    b,
		limit:      l,
		orders:     orders,
		dailyLimit: limitCfg.DailyLimit,
		now:        time.Now,
		newID:      func() string { return uuid.NewString() },
	}
}

type rejection struct{ result order.ProcessResult }

func (r *rejection) Error() string { return "rejected: " + r.result.Reason }

func (s *processorServiceImpl) Process(ctx context.Context, idempotencyKey string, o order.Order) order.ProcessResult {
	if idempotencyKey == "" {
		return order.ProcessResult{Status: order.StatusError, Reason: "idempotency_key is required"}
	}
	if err := ctx.Err(); err != nil {
		return order.ProcessResult{Status: order.StatusError, IdempotencyKey: idempotencyKey, Reason: "request canceled"}
	}
	if pre := s.validator.Validate(ctx, o); !pre.Valid {
		return order.ProcessResult{Status: order.StatusRejected, IdempotencyKey: idempotencyKey, ValidationErrors: pre.Errors}
	}
	return s.execute(ctx, idempotencyKey, o)
}

func (s *processorServiceImpl) execute(ctx context.Context, key string, o order.Order) order.ProcessResult {
	var out order.ProcessResult

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := txutil.WithTx(ctx, tx)

		if prior, dup, err := s.findPriorOrder(txCtx, key); err != nil {
			return fmt.Errorf("idempotency lookup: %w", err)
		} else if dup {
			out = prior
			return nil
		}

		newBalance, err := s.applyBalance(txCtx, o)
		if err != nil {
			return err
		}

		remaining, err := s.applyDailyLimit(txCtx, o)
		if err != nil {
			return err
		}

		orderID := s.newID()
		if err := s.writeAuditRow(txCtx, orderID, key, o, newBalance); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}

		out = order.ProcessResult{
			Status:         order.StatusFilled,
			OrderID:        orderID,
			IdempotencyKey: key,
			NewBalance:     &newBalance,
			DailyRemaining: &remaining,
		}
		return nil
	})

	if err != nil {
		var rej *rejection
		if errors.As(err, &rej) {
			rej.result.IdempotencyKey = key
			return rej.result
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return order.ProcessResult{Status: order.StatusError, IdempotencyKey: key, Reason: "request canceled"}
		}
		return order.ProcessResult{Status: order.StatusError, IdempotencyKey: key, Reason: "persistence error"}
	}
	return out
}

func (s *processorServiceImpl) findPriorOrder(ctx context.Context, key string) (order.ProcessResult, bool, error) {
	row, err := s.orders.FindPrior(ctx, key)
	if err != nil {
		return order.ProcessResult{}, false, err
	}
	if row == nil {
		return order.ProcessResult{}, false, nil
	}
	bal := row.NewBalance
	return order.ProcessResult{
		Status:         order.StatusDuplicate,
		OrderID:        row.ID,
		IdempotencyKey: row.IdempotencyKey,
		NewBalance:     &bal,
	}, true, nil
}

func (s *processorServiceImpl) applyBalance(ctx context.Context, o order.Order) (decimal.Decimal, error) {
	newBalance, err := s.balance.Apply(ctx, o)
	if err != nil {
		if errors.Is(err, repositories.ErrCustomerNotFound) {
			return decimal.Zero, &rejection{result: order.ProcessResult{
				Status: order.StatusRejected,
				Reason: "customer not found",
				ValidationErrors: []order.Error{{
					Code:    balance.CodeCustomerNotFound,
					Field:   "customer_id",
					Message: "customer not found",
				}},
			}}
		}
		if strings.HasPrefix(err.Error(), balance.CodeInsufficientBalance) {
			return decimal.Zero, &rejection{result: order.ProcessResult{
				Status: order.StatusRejected,
				Reason: "insufficient balance",
				ValidationErrors: []order.Error{{
					Code:    balance.CodeInsufficientBalance,
					Message: "insufficient balance",
				}},
			}}
		}
		return decimal.Zero, fmt.Errorf("balance: %w", err)
	}
	return newBalance, nil
}

func (s *processorServiceImpl) applyDailyLimit(ctx context.Context, o order.Order) (decimal.Decimal, error) {
	remaining, err := s.limit.Apply(ctx, o)
	if err != nil {
		if strings.HasPrefix(err.Error(), limit.CodeDailyLimitExceeded) {
			return decimal.Zero, &rejection{result: order.ProcessResult{
				Status:         order.StatusRejected,
				Reason:         "daily limit exceeded",
				DailyRemaining: &remaining,
				ValidationErrors: []order.Error{{
					Code:    limit.CodeDailyLimitExceeded,
					Field:   "quantity",
					Message: "daily limit exceeded",
				}},
			}}
		}
		return decimal.Zero, fmt.Errorf("limit: %w", err)
	}
	return remaining, nil
}

func (s *processorServiceImpl) writeAuditRow(ctx context.Context, id, key string, o order.Order, newBalance decimal.Decimal) error {
	return s.orders.Create(ctx, &models.OrderModel{
		ID:             id,
		CustomerID:     o.CustomerID,
		OrderType:      string(o.OrderType),
		Quantity:       o.Quantity,
		QuotedPrice:    o.QuotedPrice,
		Total:          o.Quantity.Mul(o.QuotedPrice),
		NewBalance:     newBalance,
		IdempotencyKey: key,
		CreatedAt:      s.now().UTC(),
	})
}

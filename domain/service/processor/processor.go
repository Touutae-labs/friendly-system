package processor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/repositories"
	validatorsvc "github.com/Touutae-labs/friendly-system/domain/service/validator"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Status string

const (
	StatusFilled    Status = "filled"
	StatusRejected  Status = "rejected"
	StatusDuplicate Status = "duplicate"
	StatusError     Status = "error"
)

type Result struct {
	Status           Status           `json:"status"`
	OrderID          string           `json:"order_id,omitempty"`
	IdempotencyKey   string           `json:"idempotency_key"`
	NewBalance       *decimal.Decimal `json:"new_balance,omitempty"`
	DailyRemaining   *decimal.Decimal `json:"daily_remaining,omitempty"`
	ValidationErrors []order.Error    `json:"validation_errors,omitempty"`
	Reason           string           `json:"reason,omitempty"`
}

type Service struct {
	db         *gorm.DB
	validator  *validatorsvc.Service
	balance    *balance.Module
	limit      *limit.Module
	orders     repositories.OrderRepository
	dailyLimit decimal.Decimal
	now        func() time.Time
	newID      func() string
}

func New(
	db *gorm.DB,
	validator *validatorsvc.Service,
	balance *balance.Module,
	limit *limit.Module,
	orders repositories.OrderRepository,
	limitCfg limit.Config,
) *Service {
	return &Service{
		db:         db,
		validator:  validator,
		balance:    balance,
		limit:      limit,
		orders:     orders,
		dailyLimit: limitCfg.DailyLimit,
		now:        time.Now,
		newID:      func() string { return uuid.NewString() },
	}
}

type rejection struct{ result Result }

func (r *rejection) Error() string { return "rejected: " + r.result.Reason }

func (s *Service) Process(ctx context.Context, idempotencyKey string, o order.Order) Result {
	if idempotencyKey == "" {
		return Result{Status: StatusError, Reason: "idempotency_key is required"}
	}
	if pre := s.validator.Validate(ctx, o); !pre.Valid {
		return Result{Status: StatusRejected, IdempotencyKey: idempotencyKey, ValidationErrors: pre.Errors}
	}
	return s.execute(ctx, idempotencyKey, o)
}

func (s *Service) execute(ctx context.Context, key string, o order.Order) Result {
	var out Result

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if prior, dup, err := s.findPriorOrder(ctx, tx, key); err != nil {
			return fmt.Errorf("idempotency lookup: %w", err)
		} else if dup {
			out = prior
			return nil
		}

		newBalance, err := s.applyBalance(ctx, tx, o)
		if err != nil {
			return err
		}

		remaining, err := s.applyDailyLimit(ctx, tx, o)
		if err != nil {
			return err
		}

		orderID := s.newID()
		if err := s.writeAuditRow(ctx, tx, orderID, key, o, newBalance); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}

		out = Result{
			Status:         StatusFilled,
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
			return Result{Status: StatusError, IdempotencyKey: key, Reason: "request canceled"}
		}
		return Result{Status: StatusError, IdempotencyKey: key, Reason: "persistence error"}
	}
	return out
}

func (s *Service) findPriorOrder(ctx context.Context, tx *gorm.DB, key string) (Result, bool, error) {
	row, err := s.orders.FindPrior(ctx, tx, key)
	if err != nil {
		return Result{}, false, err
	}
	if row == nil {
		return Result{}, false, nil
	}
	bal := row.NewBalance
	return Result{
		Status:         StatusDuplicate,
		OrderID:        row.ID,
		IdempotencyKey: row.IdempotencyKey,
		NewBalance:     &bal,
	}, true, nil
}

func (s *Service) applyBalance(ctx context.Context, tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	newBalance, err := s.balance.Apply(ctx, tx, o)
	if err != nil {
		if errors.Is(err, balance.ErrCustomerNotFound) {
			return decimal.Zero, &rejection{result: Result{
				Status: StatusRejected,
				Reason: "customer not found",
				ValidationErrors: []order.Error{{
					Code:    balance.CodeCustomerNotFound,
					Field:   "customer_id",
					Message: "customer not found",
				}},
			}}
		}
		if strings.HasPrefix(err.Error(), balance.CodeInsufficientBalance) {
			return decimal.Zero, &rejection{result: Result{
				Status: StatusRejected,
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

func (s *Service) applyDailyLimit(ctx context.Context, tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	remaining, err := s.limit.Apply(ctx, tx, o)
	if err != nil {
		if strings.HasPrefix(err.Error(), limit.CodeDailyLimitExceeded) {
			return decimal.Zero, &rejection{result: Result{
				Status:         StatusRejected,
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

func (s *Service) writeAuditRow(ctx context.Context, tx *gorm.DB, id, key string, o order.Order, newBalance decimal.Decimal) error {
	return s.orders.Create(ctx, tx, &repositories.OrderModel{
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

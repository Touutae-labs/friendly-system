package processor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/repositories"
	validatorsvc "github.com/Touutae-labs/friendly-system/domain/service/validator"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	dailyLimit decimal.Decimal
	now        func() time.Time
	newID      func() string
}

func New(db *gorm.DB, validator *validatorsvc.Service, limitCfg limit.Config) *Service {
	return &Service{
		db:         db,
		validator:  validator,
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
	if pre := s.validator.Validate(o); !pre.Valid {
		return Result{Status: StatusRejected, IdempotencyKey: idempotencyKey, ValidationErrors: pre.Errors}
	}
	return s.execute(ctx, idempotencyKey, o)
}

func (s *Service) execute(ctx context.Context, key string, o order.Order) Result {
	var out Result

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if prior, dup, err := s.findPriorOrder(tx, key); err != nil {
			return fmt.Errorf("idempotency lookup: %w", err)
		} else if dup {
			out = prior
			return nil
		}

		newBalance, err := s.applyBalance(tx, o)
		if err != nil {
			return err
		}

		remaining, err := s.applyDailyLimit(tx, o)
		if err != nil {
			return err
		}

		orderID := s.newID()
		if err := s.writeAuditRow(tx, orderID, key, o, newBalance); err != nil {
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

func (s *Service) findPriorOrder(tx *gorm.DB, key string) (Result, bool, error) {
	var row repositories.OrderModel
	err := tx.Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	bal := row.NewBalance
	return Result{
		Status:         StatusDuplicate,
		OrderID:        row.ID,
		IdempotencyKey: row.IdempotencyKey,
		NewBalance:     &bal,
	}, true, nil
}

func (s *Service) applyBalance(tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	var acct repositories.AccountModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("customer_id = ?", o.CustomerID).
		First(&acct).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
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
	if err != nil {
		return decimal.Zero, fmt.Errorf("lock account: %w", err)
	}

	cost := o.Quantity.Mul(o.QuotedPrice)
	var newBalance decimal.Decimal
	switch o.OrderType {
	case order.Buy:
		if acct.Balance.LessThan(cost) {
			return decimal.Zero, &rejection{result: Result{
				Status: StatusRejected,
				Reason: "insufficient balance",
				ValidationErrors: []order.Error{{
					Code: balance.CodeInsufficientBalance,
					Message: fmt.Sprintf("balance %s THB is less than required %s THB",
						acct.Balance.StringFixed(2), cost.StringFixed(2)),
				}},
			}}
		}
		newBalance = acct.Balance.Sub(cost)
	case order.Sell:
		newBalance = acct.Balance.Add(cost)
	default:
		return decimal.Zero, fmt.Errorf("unexpected order_type %q after validation", o.OrderType)
	}

	if err := tx.Model(&repositories.AccountModel{}).
		Where("customer_id = ?", o.CustomerID).
		Update("balance", newBalance).Error; err != nil {
		return decimal.Zero, fmt.Errorf("update balance: %w", err)
	}
	return newBalance, nil
}

func (s *Service) applyDailyLimit(tx *gorm.DB, o order.Order) (decimal.Decimal, error) {
	dayKey := s.now().UTC().Format(time.DateOnly)
	var dt repositories.DailyTotalModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("customer_id = ? AND day = ?", o.CustomerID, dayKey).
		First(&dt).Error
	current := decimal.Zero
	if err == nil {
		current = dt.Total
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, fmt.Errorf("read daily total: %w", err)
	}

	next := current.Add(o.Quantity)
	if next.GreaterThan(s.dailyLimit) {
		remainingBefore := decimal.Max(s.dailyLimit.Sub(current), decimal.Zero)
		return decimal.Zero, &rejection{result: Result{
			Status:         StatusRejected,
			Reason:         "daily limit exceeded",
			DailyRemaining: &remainingBefore,
			ValidationErrors: []order.Error{{
				Code:  limit.CodeDailyLimitExceeded,
				Field: "quantity",
				Message: fmt.Sprintf("order quantity %s exceeds remaining daily allowance %s baht-weight",
					o.Quantity.String(), remainingBefore.String()),
			}},
		}}
	}

	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"total"}),
	}).Create(&repositories.DailyTotalModel{
		CustomerID: o.CustomerID,
		Day:        dayKey,
		Total:      next,
	}).Error; err != nil {
		return decimal.Zero, fmt.Errorf("upsert daily total: %w", err)
	}
	return decimal.Max(s.dailyLimit.Sub(next), decimal.Zero), nil
}

func (s *Service) writeAuditRow(tx *gorm.DB, id, key string, o order.Order, newBalance decimal.Decimal) error {
	return tx.Create(&repositories.OrderModel{
		ID:             id,
		CustomerID:     o.CustomerID,
		OrderType:      string(o.OrderType),
		Quantity:       o.Quantity,
		QuotedPrice:    o.QuotedPrice,
		Total:          o.Quantity.Mul(o.QuotedPrice),
		NewBalance:     newBalance,
		IdempotencyKey: key,
		CreatedAt:      s.now().UTC(),
	}).Error
}

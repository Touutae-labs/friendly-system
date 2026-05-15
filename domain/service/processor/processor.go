package processor

import (
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

func (s *Service) Process(idempotencyKey string, o order.Order) Result {
	res := Result{IdempotencyKey: idempotencyKey}

	if idempotencyKey == "" {
		res.Status = StatusError
		res.Reason = "idempotency_key is required"
		return res
	}

	if prior, ok, err := s.lookupPrior(idempotencyKey); err != nil {
		res.Status = StatusError
		res.Reason = "idempotency lookup failed"
		return res
	} else if ok {
		prior.Status = StatusDuplicate
		return prior
	}

	preflight := s.validator.Validate(o)
	if !preflight.Valid {
		res.Status = StatusRejected
		res.ValidationErrors = preflight.Errors
		return res
	}

	cost := o.Quantity.Mul(o.QuotedPrice)

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		var acct repositories.AccountModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ?", o.CustomerID).
			First(&acct).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res.Status = StatusRejected
			res.Reason = "customer not found"
			res.ValidationErrors = []order.Error{{Code: balance.CodeCustomerNotFound, Field: "customer_id", Message: "customer not found"}}
			return errStopTx
		}
		if err != nil {
			return fmt.Errorf("lock account: %w", err)
		}

		bal, err := decimal.NewFromString(acct.Balance)
		if err != nil {
			return fmt.Errorf("parse balance: %w", err)
		}

		var newBalance decimal.Decimal
		switch o.OrderType {
		case order.Buy:
			if bal.LessThan(cost) {
				res.Status = StatusRejected
				res.Reason = "insufficient balance"
				res.ValidationErrors = []order.Error{{Code: balance.CodeInsufficientBalance, Message: fmt.Sprintf("balance %s THB is less than required %s THB", bal.StringFixed(2), cost.StringFixed(2))}}
				return errStopTx
			}
			newBalance = bal.Sub(cost)
		case order.Sell:
			newBalance = bal.Add(cost)
		default:
			return fmt.Errorf("unexpected order_type %q after validation", o.OrderType)
		}

		if err := tx.Model(&repositories.AccountModel{}).
			Where("customer_id = ?", o.CustomerID).
			Update("balance", newBalance.String()).Error; err != nil {
			return fmt.Errorf("update balance: %w", err)
		}

		orderID := s.newID()
		now := s.now()
		auditRow := repositories.OrderModel{
			ID:             orderID,
			CustomerID:     o.CustomerID,
			OrderType:      string(o.OrderType),
			Quantity:       o.Quantity.String(),
			QuotedPrice:    o.QuotedPrice.String(),
			Total:          cost.String(),
			NewBalance:     newBalance.String(),
			IdempotencyKey: idempotencyKey,
			CreatedAt:      now.UTC().Format(time.RFC3339Nano),
		}
		if err := tx.Create(&auditRow).Error; err != nil {
			return fmt.Errorf("insert order: %w", err)
		}

		dayKey := now.UTC().Format("2006-01-02")
		var dt repositories.DailyTotalModel
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ? AND day = ?", o.CustomerID, dayKey).
			First(&dt).Error
		current := decimal.Zero
		if err == nil {
			parsed, perr := decimal.NewFromString(dt.Total)
			if perr != nil {
				return fmt.Errorf("parse daily total: %w", perr)
			}
			current = parsed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read daily total: %w", err)
		}
		nextTotal := current.Add(o.Quantity)
		if nextTotal.GreaterThan(s.dailyLimit) {
			remainingBefore := s.dailyLimit.Sub(current)
			if remainingBefore.IsNegative() {
				remainingBefore = decimal.Zero
			}
			res.Status = StatusRejected
			res.Reason = "daily limit exceeded"
			res.DailyRemaining = &remainingBefore
			res.ValidationErrors = []order.Error{{
				Code:    limit.CodeDailyLimitExceeded,
				Field:   "quantity",
				Message: fmt.Sprintf("order quantity %s exceeds remaining daily allowance %s baht-weight", o.Quantity.String(), remainingBefore.String()),
			}}
			return errStopTx
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
			DoUpdates: clause.AssignmentColumns([]string{"total"}),
		}).Create(&repositories.DailyTotalModel{
			CustomerID: o.CustomerID,
			Day:        dayKey,
			Total:      nextTotal.String(),
		}).Error; err != nil {
			return fmt.Errorf("upsert daily total: %w", err)
		}

		res.Status = StatusFilled
		res.OrderID = orderID
		res.NewBalance = &newBalance
		remaining := s.dailyLimit.Sub(nextTotal)
		if remaining.IsNegative() {
			remaining = decimal.Zero
		}
		res.DailyRemaining = &remaining
		return nil
	})

	if txErr != nil && !errors.Is(txErr, errStopTx) {
		res.Status = StatusError
		res.Reason = "persistence error"
	}
	return res
}

func (s *Service) lookupPrior(key string) (Result, bool, error) {
	var row repositories.OrderModel
	err := s.db.Where("idempotency_key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	bal, _ := decimal.NewFromString(row.NewBalance)
	return Result{
		Status:         StatusFilled,
		OrderID:        row.ID,
		IdempotencyKey: row.IdempotencyKey,
		NewBalance:     &bal,
	}, true, nil
}

var errStopTx = errors.New("stop tx")

package validatorsvc

import (
	"context"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	"github.com/Touutae-labs/friendly-system/internal/modules/balance"
	"github.com/Touutae-labs/friendly-system/internal/modules/limit"
	"github.com/Touutae-labs/friendly-system/internal/modules/orderval"
	"github.com/Touutae-labs/friendly-system/internal/modules/quote"
)

type ValidatorService interface {
	Validate(ctx context.Context, o order.Order) order.Result
}

type validatorServiceImpl struct {
	orderval orderval.Validator
	quote    quote.Validator
	balance  balance.Module
	limit    limit.Module
}

func New(ov orderval.Validator, q quote.Validator, b balance.Module, l limit.Module) ValidatorService {
	return &validatorServiceImpl{orderval: ov, quote: q, balance: b, limit: l}
}

func (s *validatorServiceImpl) Validate(ctx context.Context, o order.Order) order.Result {
	res := order.Result{Valid: true}

	s.orderval.Apply(o, &res)
	if !res.Valid {
		return res
	}

	if ok := s.quote.Apply(ctx, o, &res); !ok {
		return res
	}

	if o.OrderType == order.Buy {
		s.balance.Validate(ctx, o, &res)
	}

	s.limit.Validate(ctx, o, &res)

	return res
}

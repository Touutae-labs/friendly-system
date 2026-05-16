package validatorsvc

import (
	"context"

	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/orderval"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
)

type Service struct {
	orderval *orderval.Validator
	quote    *quote.Validator
	balance  *balance.Module
	limit    *limit.Module
}

func New(
	ov *orderval.Validator,
	q *quote.Validator,
	b *balance.Module,
	l *limit.Module,
) *Service {
	return &Service{
		orderval: ov,
		quote:    q,
		balance:  b,
		limit:    l,
	}
}

func (s *Service) Validate(ctx context.Context, o order.Order) order.Result {
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

	if s.limit != nil {
		s.limit.Validate(ctx, o, &res)
	}

	return res
}

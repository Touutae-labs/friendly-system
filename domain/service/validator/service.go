package validatorsvc

import (
	"github.com/Touutae-labs/friendly-system/domain/common/order"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/orderval"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
)

type Service struct {
	orderval *orderval.Validator
	quote    *quote.Validator
	balance  *balance.Validator
	limit    *limit.Validator
}

func New(
	ov *orderval.Validator,
	q *quote.Validator,
	b *balance.Validator,
	l *limit.Validator,
) *Service {
	return &Service{
		orderval: ov,
		quote:    q,
		balance:  b,
		limit:    l,
	}
}

func (s *Service) Validate(o order.Order) order.Result {
	res := order.Result{Valid: true}

	s.orderval.Apply(o, &res)
	if !res.Valid {
		return res
	}

	if ok := s.quote.Apply(o, &res); !ok {
		return res
	}

	if o.OrderType == order.Buy {
		s.balance.Apply(o, &res)
	}

	if s.limit != nil {
		s.limit.Apply(o, &res)
	}

	return res
}

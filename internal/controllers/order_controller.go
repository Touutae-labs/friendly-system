package controller

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/Touutae-labs/friendly-system/internal/domains/order"
	processor "github.com/Touutae-labs/friendly-system/internal/service/processor"
	validatorsvc "github.com/Touutae-labs/friendly-system/internal/service/validator"
)

type ValidateOrderRequest struct {
	Body order.Order
}

type ProcessOrderRequest struct {
	IdempotencyKey string `header:"Idempotency-Key" required:"true"`
	Body           order.Order
}

type OrderController struct {
	validator validatorsvc.ValidatorService
	processor processor.ProcessorService
}

func NewOrderController(v validatorsvc.ValidatorService, p processor.ProcessorService) *OrderController {
	return &OrderController{validator: v, processor: p}
}

func (c *OrderController) Validate(ctx context.Context, req *ValidateOrderRequest) (*Response[order.Result], error) {
	result := c.validator.Validate(ctx, req.Body)

	attrs := []any{
		slog.String("endpoint", "validate"),
		slog.String("customer_id", req.Body.CustomerID),
		slog.String("order_type", string(req.Body.OrderType)),
		slog.String("quantity", req.Body.Quantity.String()),
		slog.String("quoted_price", req.Body.QuotedPrice.String()),
		slog.Bool("valid", result.Valid),
	}
	if len(result.Errors) > 0 {
		attrs = append(attrs, slog.Any("error_codes", errorCodes(result.Errors)))
	}
	if result.DailyRemaining != nil {
		attrs = append(attrs, slog.String("daily_remaining", result.DailyRemaining.String()))
	}
	if result.Spread != nil {
		attrs = append(attrs, slog.String("spread", result.Spread.String()))
	}
	slog.InfoContext(ctx, "order.validate", attrs...)

	return &Response[order.Result]{Body: result}, nil
}

func (c *OrderController) Process(ctx context.Context, req *ProcessOrderRequest) (*Response[order.ProcessResult], error) {
	result := c.processor.Process(ctx, req.IdempotencyKey, req.Body)

	attrs := []any{
		slog.String("endpoint", "process"),
		slog.String("idempotency_key", req.IdempotencyKey),
		slog.String("customer_id", req.Body.CustomerID),
		slog.String("order_type", string(req.Body.OrderType)),
		slog.String("quantity", req.Body.Quantity.String()),
		slog.String("quoted_price", req.Body.QuotedPrice.String()),
		slog.String("status", string(result.Status)),
	}
	if result.OrderID != "" {
		attrs = append(attrs, slog.String("order_id", result.OrderID))
	}
	if result.NewBalance != nil {
		attrs = append(attrs, slog.String("new_balance", result.NewBalance.String()))
	}
	if result.DailyRemaining != nil {
		attrs = append(attrs, slog.String("daily_remaining", result.DailyRemaining.String()))
	}
	if result.Reason != "" {
		attrs = append(attrs, slog.String("reason", result.Reason))
	}
	if len(result.ValidationErrors) > 0 {
		attrs = append(attrs, slog.Any("error_codes", errorCodes(result.ValidationErrors)))
	}

	switch result.Status {
	case order.StatusFilled, order.StatusDuplicate:
		slog.InfoContext(ctx, "order.process", attrs...)
		return &Response[order.ProcessResult]{Body: result}, nil
	case order.StatusRejected:
		slog.WarnContext(ctx, "order.process", attrs...)
		return nil, huma.NewError(http.StatusUnprocessableEntity, result.Reason)
	default:
		slog.ErrorContext(ctx, "order.process", attrs...)
		return nil, huma.NewError(http.StatusInternalServerError, "internal error")
	}
}

func errorCodes(errs []order.Error) []string {
	codes := make([]string, 0, len(errs))
	for _, e := range errs {
		codes = append(codes, e.Code)
	}
	return codes
}

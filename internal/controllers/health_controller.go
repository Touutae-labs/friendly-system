package controller

import "context"

type HealthResponseBody struct {
	Status string `json:"status"`
}

type HealthController struct{}

func NewHealthController() *HealthController { return &HealthController{} }

func (c *HealthController) Healthz(ctx context.Context, req *EmptyRequest) (*Response[HealthResponseBody], error) {
	return &Response[HealthResponseBody]{Body: HealthResponseBody{Status: "ok"}}, nil
}

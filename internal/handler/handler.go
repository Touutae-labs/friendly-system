package handler

import (
	"github.com/danielgtaylor/huma/v2"

	controller "github.com/Touutae-labs/friendly-system/internal/controllers"
)

func RegisterHandlers(api huma.API, c *controller.Controllers) {
	registerHealthHandlers(api, c)
	registerOrderHandlers(api, c)
}

func registerHealthHandlers(api huma.API, c *controller.Controllers) {
	tags := []string{"Health"}

	huma.Register(api, huma.Operation{
		Path:   "/healthz",
		Method: "GET",
		Tags:   tags,
	}, c.HealthController.Healthz)
}

func registerOrderHandlers(api huma.API, c *controller.Controllers) {
	tags := []string{"Orders"}

	huma.Register(api, huma.Operation{
		Path:   "/orders/validate",
		Method: "POST",
		Tags:   tags,
	}, c.OrderController.Validate)

	huma.Register(api, huma.Operation{
		Path:   "/orders/process",
		Method: "POST",
		Tags:   tags,
	}, c.OrderController.Process)
	// ... add more order routes here
}

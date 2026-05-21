package controller

type Controllers struct {
	HealthController *HealthController
	OrderController  *OrderController
}

func NewControllers(health *HealthController, order *OrderController) *Controllers {
	return &Controllers{HealthController: health, OrderController: order}
}

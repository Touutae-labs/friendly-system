package controller

type Response[T any] struct {
	Body T
}

type RequestWithBody[T any] struct {
	Body T `validate:"required"`
}

type EmptyRequest struct{}

type EmptyResponse struct {
	Status int
}

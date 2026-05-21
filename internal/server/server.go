package server

import (
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

type ServerTitle string
type ServerVersion string

type ServerConfig struct {
	Title            ServerTitle
	Version          ServerVersion
	Port             string
	MaxPayloadSizeKB int
	TimeoutSeconds   int
	BaseURL          string
}

type Server struct {
	App    *fiber.App
	API    *huma.API
	Config *ServerConfig
}

func NewServer(cfg ServerConfig) *Server {
	humaConfig := huma.DefaultConfig(string(cfg.Title), string(cfg.Version))

	timeout := 30 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	bodyLimit := 4 * 1024 * 1024
	if cfg.MaxPayloadSizeKB > 0 {
		bodyLimit = cfg.MaxPayloadSizeKB * 1024
	}

	app := fiber.New(fiber.Config{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		BodyLimit:    bodyLimit,
	})

	api := humafiber.New(app, humaConfig)

	app.Use(logger.New())

	if cfg.BaseURL != "" {
		app.Get("/docs", func(c fiber.Ctx) error {
			html := fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <title>%s API Reference</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', { url: '%s/openapi.json' })
    </script>
  </body>
</html>`, string(cfg.Title), cfg.BaseURL)
			return c.Type("html").SendString(html)
		})
	}

	return &Server{
		App:    app,
		API:    &api,
		Config: &cfg,
	}
}

func (s *Server) Start() error {
	return s.App.Listen(":" + s.Config.Port)
}

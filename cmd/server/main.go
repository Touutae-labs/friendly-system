package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/Touutae-labs/friendly-system/internal/configurations"
	"github.com/Touutae-labs/friendly-system/internal/di"
	"github.com/Touutae-labs/friendly-system/internal/server"
)

var Version = "dev"

const serverName = "friendly-system"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Structured JSON logging for the whole app — controllers and services
	// pick this up via slog.InfoContext / slog.ErrorContext.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	configPath := os.Getenv("APP_CONFIG")
	if configPath == "" {
		configPath = "config.yml"
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		log.Fatalf("error loading config from %s: %v", configPath, err)
	}

	var cfg configurations.Config
	if err := k.Unmarshal("", &cfg); err != nil {
		log.Fatalf("error parsing config: %v", err)
	}

	cfg.Server.Title = serverName
	cfg.Server.Version = Version

	srv, cleanup, err := di.Initialize(cfg, server.ServerTitle(serverName), server.ServerVersion(Version))
	if err != nil {
		log.Fatalf("error initializing server: %v", err)
	}
	defer cleanup()

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("error starting server: %v", err)
		}
	}()

	<-ctx.Done()

	log.Print("shutdown requested")
	if err := srv.App.Shutdown(); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	log.Print("gracefully shutdown")
}

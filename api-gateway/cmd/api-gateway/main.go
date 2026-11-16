package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ecommerce.local/api-gateway/internal/bootstrap"
	"ecommerce.local/api-gateway/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err, "service", "api-gateway")
		os.Exit(1)
	}

	slog.Info("starting api-gateway",
		"http_port", cfg.HTTPPort,
		"service", "api-gateway",
	)

	app, err := bootstrap.New(cfg)
	if err != nil {
		slog.Error("failed creating application", "error", err, "service", "api-gateway")
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	app.Run(errCh)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal", "signal", sig.String(), "service", "api-gateway")
	case srvErr := <-errCh:
		slog.Error("server error", "error", srvErr, "service", "api-gateway")
	}

	app.Health.SetReady(false)
	cancel()

	if err := app.Shutdown(context.WithoutCancel(rootCtx)); err != nil {
		slog.Error("application shutdown error", "error", err, "service", "api-gateway")
	}

	slog.Info("shutdown complete", "service", "api-gateway")
}

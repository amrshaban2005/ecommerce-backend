package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ecommerce.local/auth-service/internal/bootstrap"
	"ecommerce.local/auth-service/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err, "service", "auth-service")
		os.Exit(1)
	}

	slog.Info("starting auth-service",
		"http_port", cfg.HTTPPort,
		"grpc_port", cfg.GRPCPort,
		"service", "auth-service",
	)

	app, err := bootstrap.New(cfg)
	if err != nil {
		slog.Error("failed creating application", "error", err, "service", "auth-service")
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	app.Run(errCh)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal", "signal", sig.String(), "service", "auth-service")
	case srvErr := <-errCh:
		slog.Error("server error", "error", srvErr, "service", "auth-service")
	}

	app.Health.SetReady(false)
	cancel()

	if err := app.Shutdown(context.WithoutCancel(rootCtx)); err != nil {
		slog.Error("application shutdown error", "error", err, "service", "auth-service")
	}

	slog.Info("shutdown complete", "service", "auth-service")
}

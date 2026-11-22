package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ecommerce.local/order-service/internal/bootstrap"
	"ecommerce.local/order-service/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err, "service", "order-service")
		os.Exit(1)
	}

	slog.Info("starting order-service",
		"http_port", cfg.HTTPPort,
		"grpc_port", cfg.GRPCPort,
		"service", "order-service",
	)

	app, err := bootstrap.New(cfg)
	if err != nil {
		slog.Error("failed creating application", "error", err, "service", "order-service")
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
		slog.Info("shutdown signal", "signal", sig.String(), "service", "order-service")
	case srvErr := <-errCh:
		slog.Error("server error", "error", srvErr, "service", "order-service")
	}

	app.Health.SetReady(false)
	cancel()

	if err := app.Shutdown(context.WithoutCancel(rootCtx)); err != nil {
		slog.Error("application shutdown error", "error", err, "service", "order-service")
	}

	slog.Info("shutdown complete", "service", "order-service")
}

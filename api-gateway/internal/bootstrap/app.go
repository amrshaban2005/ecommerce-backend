// Package bootstrap wires the gateway and manages its lifecycle.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	grpcadapter "ecommerce.local/api-gateway/internal/adapter/grpc"
	"ecommerce.local/api-gateway/internal/config"
	"ecommerce.local/api-gateway/internal/health"
	"ecommerce.local/api-gateway/internal/metrics"
	"ecommerce.local/api-gateway/internal/middleware"
	"ecommerce.local/api-gateway/internal/proxy"
	"ecommerce.local/api-gateway/internal/ratelimit"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Config      *config.Config
	HTTPServer  *http.Server
	Health      *health.Handler
	RedisClient *redis.Client
	AuthClient  *grpcadapter.Client
}

func New(cfg *config.Config) (*Application, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr(), ContextTimeoutEnabled: true})
	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(startupCtx).Err(); err != nil {
		slog.Warn("redis not reachable, rate limiting will fail-open", "error", err, "service", "api-gateway")
	}
	cancel()
	limiter := ratelimit.New(rdb, cfg.RateLimitPerIP)
	upstreamProxy, err := proxy.New(cfg.Routes(), limiter, proxy.WithUpstreamTimeout(time.Duration(cfg.UpstreamTimeoutSeconds)*time.Second))
	if err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("proxy routes: %w", err)
	}
	auth, err := grpcadapter.NewAuthClient(cfg.AuthGRPCAddress, time.Duration(cfg.AuthVerifyTimeoutSeconds)*time.Second)
	if err != nil {
		_ = rdb.Close()
		return nil, err
	}
	hh := health.New()
	router := mux.NewRouter()
	router.Use(handlers.ProxyHeaders)
	router.Use(middleware.CorrelationIDMiddleware)
	router.Use(middleware.RequestLogger)
	router.Use(handlers.RecoveryHandler())
	router.Use(middleware.Auth(auth))
	router.HandleFunc("/health/liveness", hh.Liveness).Methods(http.MethodGet)
	router.HandleFunc("/health/readiness", hh.Readiness).Methods(http.MethodGet)
	router.Handle("/metrics", metrics.Handler())
	router.PathPrefix("/").Handler(middleware.RequestSizeLimit(cfg.MaxRequestBodyBytes)(upstreamProxy))

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &Application{Config: cfg, HTTPServer: httpSrv, Health: hh, RedisClient: rdb, AuthClient: auth}, nil
}

func (a *Application) Run(errCh chan<- error) {
	go func() {
		slog.Info("http server starting", "port", a.Config.HTTPPort, "service", "api-gateway")
		if err := a.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
}

func (a *Application) Shutdown(ctx context.Context) error {
	a.Health.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.HTTPServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown error: %w", err)
	}
	if a.RedisClient != nil {
		if err := a.RedisClient.Close(); err != nil {
			return fmt.Errorf("redis close error: %w", err)
		}
	}
	if a.AuthClient != nil {
		if err := a.AuthClient.Close(); err != nil {
			return fmt.Errorf("Auth grpc close error: %w", err)
		}
	}
	return nil
}

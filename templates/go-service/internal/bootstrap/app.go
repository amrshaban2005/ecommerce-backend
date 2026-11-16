// Package bootstrap wires the service and manages its lifecycle.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"template.local/service/internal/config"
	"template.local/service/internal/dbmigrate"
	"template.local/service/internal/health"
)

type Application struct {
	Config     *config.Config
	HTTPServer *http.Server
	GRPCServer *grpc.Server
	Health     *health.Handler
	Listener   net.Listener
	PgPool     *pgxpool.Pool
}

func New(cfg *config.Config) (*Application, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pgPool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("postgres pool: %w", err)
	}
	if err := pgPool.Ping(ctx); err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	if err := dbmigrate.Migrate(cfg.PostgresDSN); err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("postgres migrate: %w", err)
	}

	// Compose repositories -> use cases -> inbound adapters here.
	hh := health.New()

	grpcSrv := grpc.NewServer()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("grpc listen: %w", err)
	}

	router := mux.NewRouter()
	router.Use(handlers.ProxyHeaders)
	router.Use(handlers.RecoveryHandler())
	router.HandleFunc("/health/liveness", hh.Liveness).Methods(http.MethodGet)
	router.HandleFunc("/health/readiness", hh.Readiness).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Application{
		Config:     cfg,
		HTTPServer: httpSrv,
		GRPCServer: grpcSrv,
		Health:     hh,
		Listener:   listener,
		PgPool:     pgPool,
	}, nil
}

func (a *Application) Run(errCh chan<- error) {
	go func() {
		slog.Info("grpc server starting", "port", a.Config.GRPCPort, "service", "service-template")
		if err := a.GRPCServer.Serve(a.Listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	go func() {
		slog.Info("http server starting", "port", a.Config.HTTPPort, "service", "service-template")
		if err := a.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
}

func (a *Application) Shutdown(ctx context.Context) error {
	a.Health.SetReady(false)
	a.GRPCServer.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := a.HTTPServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown error: %w", err)
	}

	if a.PgPool != nil {
		a.PgPool.Close()
	}

	return nil
}

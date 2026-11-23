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

	paymentv1 "ecommerce.local/contracts/payment/v1"
	grpcadapter "ecommerce.local/payment-service/internal/adapter/grpc"
	httpadapter "ecommerce.local/payment-service/internal/adapter/http"
	"ecommerce.local/payment-service/internal/adapter/messaging"
	"ecommerce.local/payment-service/internal/adapter/repository"
	stripeadapter "ecommerce.local/payment-service/internal/adapter/stripe"
	"ecommerce.local/payment-service/internal/adapter/worker"
	"ecommerce.local/payment-service/internal/application"
	"ecommerce.local/payment-service/internal/config"
	"ecommerce.local/payment-service/internal/dbmigrate"
	"ecommerce.local/payment-service/internal/health"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

type Application struct {
	Config         *config.Config
	HTTPServer     *http.Server
	GRPCServer     *grpc.Server
	Health         *health.Handler
	Listener       net.Listener
	PgPool         *pgxpool.Pool
	RecoveryWorker *worker.RecoveryWorker
	recoveryCancel context.CancelFunc
	recoveryDone   chan struct{}
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

	provider, err := stripeadapter.NewGateway(cfg.StripeSecretKey, cfg.StripeWebhookSecret, time.Duration(cfg.StripeTimeoutSeconds)*time.Second)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("Stripe gateway: %w", err)
	}
	payments := repository.NewPostgresPaymentRepository(pgPool)
	publisher := messaging.NewPaymentPublisher(cfg.RabbitMQURL)
	events := repository.NewPostgresWebhookRepository(pgPool)
	checkout := application.NewPaymentService(payments, events, provider, provider, publisher, cfg.SuccessURL, cfg.CancelURL)
	recovery := worker.NewRecoveryWorker(checkout, 5*time.Second, 10)
	webhookHandler := httpadapter.NewWebhookHandler(checkout)
	hh := health.New()

	grpcSrv := grpc.NewServer()
	paymentv1.RegisterPaymentServiceServer(grpcSrv, grpcadapter.NewPaymentServer(checkout))

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
	router.HandleFunc("/api/v1/webhooks/stripe", webhookHandler.Receive).Methods(http.MethodPost)
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Application{
		Config:         cfg,
		HTTPServer:     httpSrv,
		GRPCServer:     grpcSrv,
		Health:         hh,
		Listener:       listener,
		PgPool:         pgPool,
		RecoveryWorker: recovery,
	}, nil
}

func (a *Application) Run(errCh chan<- error) {
	recoveryCtx, cancel := context.WithCancel(context.Background())
	a.recoveryCancel = cancel
	a.recoveryDone = make(chan struct{})
	go func() { defer close(a.recoveryDone); a.RecoveryWorker.Run(recoveryCtx) }()
	go func() {
		slog.Info("grpc server starting", "port", a.Config.GRPCPort, "service", "payment-service")
		if err := a.GRPCServer.Serve(a.Listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	go func() {
		slog.Info("http server starting", "port", a.Config.HTTPPort, "service", "payment-service")
		if err := a.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
}

func (a *Application) Shutdown(ctx context.Context) error {
	a.Health.SetReady(false)
	if a.recoveryCancel != nil {
		a.recoveryCancel()
	}
	a.GRPCServer.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var errs []error
	if err := a.HTTPServer.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, err)
		if closeErr := a.HTTPServer.Close(); closeErr != nil {
			errs = append(errs, closeErr)
		}
	}
	if a.recoveryDone != nil {
		select {
		case <-a.recoveryDone:
		case <-shutdownCtx.Done():
			errs = append(errs, shutdownCtx.Err())
		}
	}
	if a.PgPool != nil {
		a.PgPool.Close()
	}
	return errors.Join(errs...)
}

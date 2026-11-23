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

	orderv1 "ecommerce.local/contracts/order/v1"
	grpcadapter "ecommerce.local/order-service/internal/adapter/grpc"
	httpadapter "ecommerce.local/order-service/internal/adapter/http"
	"ecommerce.local/order-service/internal/adapter/messaging"
	"ecommerce.local/order-service/internal/adapter/repository"
	"ecommerce.local/order-service/internal/adapter/worker"
	"ecommerce.local/order-service/internal/application"
	"ecommerce.local/order-service/internal/config"
	"ecommerce.local/order-service/internal/dbmigrate"
	"ecommerce.local/order-service/internal/health"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

type Application struct {
	Config          *config.Config
	HTTPServer      *http.Server
	GRPCServer      *grpc.Server
	Health          *health.Handler
	Listener        net.Listener
	PgPool          *pgxpool.Pool
	CheckoutClient  *grpcadapter.CheckoutClient
	PaymentClient   *grpcadapter.PaymentClient
	RecoveryWorker  *worker.RecoveryWorker
	PaymentConsumer *messaging.PaymentConsumer
	recoveryCancel  context.CancelFunc
	recoveryDone    chan struct{}
	consumerDone    chan struct{}
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

	clients, err := grpcadapter.NewCheckoutClient(cfg.CartGRPCAddress, cfg.ProductGRPCAddress, cfg.InventoryGRPCAddress, time.Duration(cfg.RPCTimeoutSeconds)*time.Second)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("checkout clients: %w", err)
	}
	paymentClient, err := grpcadapter.NewPaymentClient(cfg.PaymentGRPCAddress, time.Duration(cfg.PaymentTimeoutSeconds)*time.Second)
	if err != nil {
		_ = clients.Close()
		pgPool.Close()
		return nil, fmt.Errorf("payment client: %w", err)
	}
	orders := repository.NewPostgresOrderRepository(pgPool)
	processingTimeout := time.Duration(cfg.CheckoutTimeoutSeconds) * time.Second
	checkout := application.NewCheckoutService(orders, clients, clients, clients, paymentClient, processingTimeout)
	recovery := worker.NewRecoveryWorker(checkout, time.Duration(cfg.RecoveryIntervalSeconds)*time.Second, cfg.RecoveryBatchSize)
	checkoutHandler := httpadapter.NewCheckoutHandler(checkout, time.Duration(cfg.CheckoutTimeoutSeconds)*time.Second)
	hh := health.New()

	grpcSrv := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(grpcSrv, grpcadapter.NewOrderServer(checkout))

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		_ = paymentClient.Close()
		_ = clients.Close()
		pgPool.Close()
		return nil, fmt.Errorf("grpc listen: %w", err)
	}

	router := mux.NewRouter()
	router.Use(handlers.ProxyHeaders)
	router.Use(handlers.RecoveryHandler())
	router.HandleFunc("/health/liveness", hh.Liveness).Methods(http.MethodGet)
	router.HandleFunc("/health/readiness", hh.Readiness).Methods(http.MethodGet)
	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/api/v1/checkout", httpadapter.RequireCustomer(http.HandlerFunc(checkoutHandler.Start))).Methods(http.MethodPost)
	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Application{
		Config:          cfg,
		HTTPServer:      httpSrv,
		GRPCServer:      grpcSrv,
		Health:          hh,
		Listener:        listener,
		PgPool:          pgPool,
		CheckoutClient:  clients,
		PaymentClient:   paymentClient,
		RecoveryWorker:  recovery,
		PaymentConsumer: messaging.NewPaymentConsumer(cfg.RabbitMQURL, checkout),
	}, nil
}

func (a *Application) Run(errCh chan<- error) {
	recoveryCtx, cancel := context.WithCancel(context.Background())
	a.recoveryCancel = cancel
	a.recoveryDone = make(chan struct{})
	a.consumerDone = make(chan struct{})
	go func() {
		defer close(a.consumerDone)
		a.PaymentConsumer.Run(recoveryCtx)
	}()
	go func() {
		defer close(a.recoveryDone)
		a.RecoveryWorker.Run(recoveryCtx)
	}()
	go func() {
		slog.Info("grpc server starting", "port", a.Config.GRPCPort, "service", "order-service")
		if err := a.GRPCServer.Serve(a.Listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc server: %w", err)
		}
	}()

	go func() {
		slog.Info("http server starting", "port", a.Config.HTTPPort, "service", "order-service")
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
		errs = append(errs, fmt.Errorf("http shutdown error: %w", err))
		// Stop remaining handlers before closing the clients and pool they use.
		if err := a.HTTPServer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("http close error: %w", err))
		}
	}

	if a.recoveryDone != nil {
		select {
		case <-a.recoveryDone:
		case <-shutdownCtx.Done():
			errs = append(errs, fmt.Errorf("checkout recovery shutdown: %w", shutdownCtx.Err()))
		}
	}
	if a.consumerDone != nil {
		select {
		case <-a.consumerDone:
		case <-shutdownCtx.Done():
			errs = append(errs, fmt.Errorf("payment consumer shutdown: %w", shutdownCtx.Err()))
		}
	}
	if a.PgPool != nil {
		a.PgPool.Close()
	}
	if a.CheckoutClient != nil {
		if err := a.CheckoutClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close checkout clients: %w", err))
		}
	}
	if a.PaymentClient != nil {
		if err := a.PaymentClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close payment client: %w", err))
		}
	}
	return errors.Join(errs...)
}

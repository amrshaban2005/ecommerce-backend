package worker

import (
	"context"
	"log/slog"
	"time"

	"ecommerce.local/order-service/internal/port"
)

// RecoveryWorker is an inbound adapter. Scheduling stays outside the use case.
type RecoveryWorker struct {
	recovery  port.CheckoutRecovery
	interval  time.Duration
	batchSize int
}

func NewRecoveryWorker(recovery port.CheckoutRecovery, interval time.Duration, batchSize int) *RecoveryWorker {
	return &RecoveryWorker{recovery: recovery, interval: interval, batchSize: batchSize}
}

func (w *RecoveryWorker) Run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := w.recovery.Recover(ctx, w.batchSize); err != nil && ctx.Err() == nil {
				slog.Warn("checkout recovery cycle failed", "error", err, "service", "order-service")
			}
			timer.Reset(w.interval)
		}
	}
}

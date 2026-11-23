package worker

import (
	"context"
	"log/slog"
	"time"

	"ecommerce.local/payment-service/internal/port"
)

// RecoveryWorker is an inbound adapter. Scheduling stays outside the use case.
type RecoveryWorker struct {
	recovery  port.PaymentRecovery
	interval  time.Duration
	batchSize int
}

func NewRecoveryWorker(recovery port.PaymentRecovery, interval time.Duration, batchSize int) *RecoveryWorker {
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
				slog.Warn("payment recovery cycle failed", "error", err, "service", "payment-service")
			}
			timer.Reset(w.interval)
		}
	}
}

package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ecommerce.local/payment-service/internal/port"
)

// Recovery handles only saved webhooks and final outcomes awaiting broker confirmation.
func (s *PaymentService) Recover(ctx context.Context, limit int) error {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	events, err := s.events.Pending(listCtx, limit)
	cancel()
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	for _, event := range events {
		if ctx.Err() != nil {
			break
		}
		workCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		err := s.processWebhook(workCtx, event)
		stop()
		if err != nil {
			errs = append(errs, fmt.Errorf("process webhook %s: %w", event.EventID, err))
		}
	}
	listCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
	payments, err := s.payments.PendingEvents(listCtx, limit)
	cancel()
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, p := range payments {
		if ctx.Err() != nil {
			break
		}
		workCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		err := s.publisher.PublishPaymentOutcome(workCtx, port.PaymentOutcome{OrderID: p.OrderID, PaymentID: p.ID, CustomerID: p.CustomerID,
			AmountUSDCents: p.AmountUSDCents, Currency: p.Currency, Status: p.Status})
		if err == nil {
			err = s.payments.MarkPublished(workCtx, p.ID, p.Status)
		}
		stop()
		if err != nil {
			retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			err = errors.Join(err, s.payments.RetryPublish(retryCtx, p.ID))
			retryCancel()
			errs = append(errs, fmt.Errorf("publish outcome for payment %s: %w", p.ID, err))
		}
	}
	return errors.Join(errs...)
}

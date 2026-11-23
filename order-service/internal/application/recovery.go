package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ecommerce.local/order-service/internal/domain"
)

// Recovery resumes the saved order. It never rereads the cart or current prices.
func (s *CheckoutService) Recover(ctx context.Context, limit int) error {
	listCtx, cancel := context.WithTimeout(ctx, s.processingTimeout)
	ids, err := s.orders.ListRecovery(listCtx, limit)
	cancel()
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		workCtx, cancel := context.WithTimeout(ctx, s.processingTimeout)
		order, err := s.orders.LoadOrder(workCtx, id)
		if err == nil {
			switch order.Status {
			case domain.OrderPending:
				_, err = s.reserveOrder(workCtx, order)
			case domain.OrderPaid:
				_, err = s.finalizeOrder(workCtx, order)
			case domain.OrderAwaitingPayment:
				_, err = s.expireOrder(workCtx, order)
			case domain.OrderCleanupPending:
				if order.PaymentID != "" {
					_, err = s.expireOrder(workCtx, order)
				} else {
					_, err = s.releaseOrder(workCtx, order)
				}
			}
		}
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("recover order %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// scheduleRetry records when the worker should try this saved order again.
// The independent context also works after an HTTP disconnect. A crash before
// this write is covered by the retry_after already stored with the order.
func (s *CheckoutService) scheduleRetry(ctx context.Context, order domain.Order, err error) (domain.Order, error) {
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	err = errors.Join(err, s.orders.RetryCheckout(retryCtx, order.ID, err.Error(), 10*time.Second))
	current, readErr := s.orders.LoadOrder(retryCtx, order.ID)
	if readErr == nil {
		// The update may already have committed before its response was lost, or
		// another processor may have finished. Return that outcome without undoing it.
		if current.Status == domain.OrderCompleted || current.Status == domain.OrderExpired || current.Status == domain.OrderCheckoutFailed {
			return current, nil
		}
		order = current
	}
	return order, err
}

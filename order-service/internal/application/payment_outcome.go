package application

import (
	"context"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"github.com/google/uuid"
)

func (s *CheckoutService) RecordPaymentOutcome(ctx context.Context, cmd port.PaymentOutcomeCommand) error {
	for field, value := range map[string]string{"order_id": cmd.OrderID, "payment_id": cmd.PaymentID, "customer_id": cmd.CustomerID} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			return &domain.ValidationError{Field: field, Message: "must be a valid non-zero UUID"}
		}
	}
	if cmd.Currency != "USD" || cmd.AmountUSDCents <= 0 {
		return &domain.ValidationError{Field: "amount_usd_cents", Message: "a positive USD amount is required"}
	}
	if cmd.Status != "PAID" && cmd.Status != "FAILED" {
		return &domain.ValidationError{Field: "status", Message: "must be PAID or FAILED"}
	}
	recorded, err := s.orders.RecordPaymentOutcome(ctx, cmd)
	if err != nil {
		return err
	}
	if !recorded {
		return domain.ErrPaymentUnavailable
	}
	return nil
}

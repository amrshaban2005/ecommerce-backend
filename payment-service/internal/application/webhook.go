package application

import (
	"context"
	"errors"
	"time"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
)

func (s *PaymentService) ReceiveWebhook(ctx context.Context, payload []byte, signature string) error {
	event, err := s.verifier.VerifyWebhook(payload, signature)
	if err != nil || event.LiveMode {
		return domain.ErrInvalidWebhook
	}
	saved, err := s.events.Save(ctx, event, payload)
	if err != nil {
		return err
	}
	if saved.Processed {
		return nil
	}
	return s.processWebhook(ctx, saved)
}

func (s *PaymentService) processWebhook(ctx context.Context, event port.StoredWebhook) error {
	session, err := s.stripe.GetCheckoutSession(ctx, event.SessionID)
	if err == nil {
		paymentID, idErr := domain.NormalizeID("payment_id", session.PaymentID)
		if idErr != nil {
			return s.events.MarkIgnored(ctx, event.EventID)
		}
		session.PaymentID = paymentID
		var payment domain.Payment
		payment, err = s.payments.FindByID(ctx, session.PaymentID)
		if errors.Is(err, domain.ErrPaymentNotFound) {
			return s.events.MarkIgnored(ctx, event.EventID)
		}
		if err == nil {
			payment, err = s.saveProviderSession(ctx, payment, session, nil)
			if err == nil {
				err = s.events.MarkProcessed(ctx, event.EventID, payment.ID)
			}
		}
	}
	if err != nil {
		retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		return errors.Join(err, s.events.Retry(retryCtx, event.EventID))
	}
	return nil
}

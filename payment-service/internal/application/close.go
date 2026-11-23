package application

import (
	"context"
	"errors"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	"github.com/google/uuid"
)

// ClosePayment never treats a timeout as failure. Only confirmed paid, confirmed
// closed/unpaid, or pending is returned to the stock-release coordinator.
func (s *PaymentService) ClosePayment(ctx context.Context, cmd port.PreparePaymentCommand) (domain.Payment, error) {
	cmd, err := normalizePaymentRequest(cmd)
	if err != nil {
		return domain.Payment{}, err
	}
	p, err := s.payments.FindByOrder(ctx, cmd.OrderID)
	if errors.Is(err, domain.ErrPaymentNotFound) {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return p, idErr
		}
		// A cancellation row wins against a late PreparePayment insert. No provider
		// operation can have started without a saved payment row first.
		p, err = s.payments.CreateOrGet(ctx, domain.Payment{ID: id.String(), OrderID: cmd.OrderID, CustomerID: cmd.CustomerID,
			ReservationID: cmd.ReservationID, ReservationExpiresAt: cmd.ReservationExpiresAt, AmountUSDCents: cmd.AmountUSDCents,
			Currency: cmd.Currency, Status: domain.PaymentFailed, ProviderIdempotencyKey: "checkout:" + id.String(), SuccessURL: s.successURL, CancelURL: s.cancelURL})
	}
	if err != nil {
		return p, err
	}
	if p.CustomerID != cmd.CustomerID || p.AmountUSDCents != cmd.AmountUSDCents || p.Currency != cmd.Currency || p.ReservationID != cmd.ReservationID || !p.ReservationExpiresAt.Equal(cmd.ReservationExpiresAt) {
		return withoutURL(p), domain.ErrPaymentMismatch
	}
	if p.Status == domain.PaymentPaid || (p.Status == domain.PaymentFailed && p.ProviderSessionID == "") {
		return withoutURL(p), nil
	}
	if p.ProviderSessionID == "" {
		// Recover the result of an in-flight/lost creation using its original key,
		// then close it. If reconciliation fails, stock must remain held.
		p, err = s.createSession(ctx, p)
		if err != nil {
			return withoutURL(p), err
		}
	}
	session, err := s.stripe.GetCheckoutSession(ctx, p.ProviderSessionID)
	if err != nil {
		return withoutURL(p), err
	}
	p, err = s.saveProviderSession(ctx, p, session, nil)
	if err != nil {
		return withoutURL(p), err
	}
	if p.Status == domain.PaymentPaid || session.Status == domain.PaymentFailed {
		return withoutURL(p), nil
	}
	if session.Payable {
		_, expireErr := s.stripe.ExpireCheckoutSession(ctx, p.ProviderSessionID)
		// Expiration can lose a race to payment completion. Always retrieve again,
		// including after an expiration error, before deciding whether to release.
		session, err = s.stripe.GetCheckoutSession(ctx, p.ProviderSessionID)
		if err != nil {
			return withoutURL(p), errors.Join(expireErr, err)
		}
		p, err = s.saveProviderSession(ctx, p, session, nil)
		if err != nil {
			return withoutURL(p), err
		}
		if p.Status == domain.PaymentPaid || session.Status == domain.PaymentFailed {
			return withoutURL(p), nil
		}
	}
	p.Status = domain.PaymentPending
	p.PaymentURL = ""
	return p, nil
}

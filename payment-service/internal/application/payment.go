package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	"github.com/google/uuid"
)

type PaymentService struct {
	payments   port.PaymentRepository
	stripe     port.StripeGateway
	events     port.WebhookRepository
	verifier   port.WebhookVerifier
	publisher  port.PaymentPublisher
	successURL string
	cancelURL  string
}

func NewPaymentService(payments port.PaymentRepository, events port.WebhookRepository, stripe port.StripeGateway, verifier port.WebhookVerifier, publisher port.PaymentPublisher, successURL, cancelURL string) port.PaymentWorkflow {
	return &PaymentService{payments: payments, events: events, stripe: stripe, verifier: verifier, publisher: publisher, successURL: successURL, cancelURL: cancelURL}
}

func (s *PaymentService) PreparePayment(ctx context.Context, cmd port.PreparePaymentCommand) (domain.Payment, error) {
	cmd, err := normalizePaymentRequest(cmd)
	if err != nil {
		return domain.Payment{}, err
	}
	a, err := s.payments.FindByOrder(ctx, cmd.OrderID)
	if errors.Is(err, domain.ErrPaymentNotFound) {
		if !cmd.ReservationExpiresAt.After(time.Now()) {
			return a, domain.ErrReservationExpired
		}
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return a, fmt.Errorf("create payment ID: %w", idErr)
		}
		a, err = s.payments.CreateOrGet(ctx, domain.Payment{
			ID: id.String(), OrderID: cmd.OrderID, CustomerID: cmd.CustomerID, ReservationID: cmd.ReservationID,
			ReservationExpiresAt: cmd.ReservationExpiresAt, AmountUSDCents: cmd.AmountUSDCents, Currency: cmd.Currency,
			ProviderIdempotencyKey: "checkout:" + id.String(), SuccessURL: s.successURL, CancelURL: s.cancelURL,
		})
	}
	if err != nil {
		return a, err
	}
	if a.CustomerID != cmd.CustomerID || a.AmountUSDCents != cmd.AmountUSDCents || a.Currency != cmd.Currency ||
		a.ReservationID != cmd.ReservationID || !a.ReservationExpiresAt.Equal(cmd.ReservationExpiresAt) {
		return a, domain.ErrPaymentMismatch
	}
	if a.Status == domain.PaymentPaid {
		return withoutURL(a), nil
	}
	// There is only one session per order. A confirmed failed session is not replaced.
	if a.Status == domain.PaymentFailed {
		return withoutURL(a), nil
	}
	if !a.ReservationExpiresAt.After(time.Now()) {
		return withoutURL(a), domain.ErrReservationExpired
	}
	if a.ProviderSessionID == "" {
		return s.createSession(ctx, a)
	}
	return s.refresh(ctx, a)
}

func (s *PaymentService) GetOrderPayment(ctx context.Context, orderID string) (domain.Payment, error) {
	orderID, err := domain.NormalizeID("order_id", orderID)
	if err != nil {
		return domain.Payment{}, err
	}
	a, err := s.payments.FindByOrder(ctx, orderID)
	if err != nil {
		return a, err
	}
	// A pending creation without a session ID cannot prove an unpaid outcome.
	if a.ProviderSessionID == "" || a.Status != domain.PaymentPending {
		return withoutURL(a), nil
	}
	return s.refresh(ctx, a)
}

func (s *PaymentService) refresh(ctx context.Context, a domain.Payment) (domain.Payment, error) {
	session, err := s.stripe.GetCheckoutSession(ctx, a.ProviderSessionID)
	return s.saveProviderSession(ctx, a, session, err)
}

func (s *PaymentService) saveProviderSession(ctx context.Context, a domain.Payment, session port.CheckoutSession, providerErr error) (domain.Payment, error) {
	if providerErr != nil {
		return withoutURL(a), providerErr
	}
	if session.LiveMode || session.PaymentID != a.ID || session.OrderID != a.OrderID || session.CustomerID != a.CustomerID ||
		session.AmountUSDCents != a.AmountUSDCents || session.Currency != a.Currency || session.SessionID == "" ||
		(a.ProviderSessionID != "" && session.SessionID != a.ProviderSessionID) {
		return withoutURL(a), domain.ErrPaymentMismatch
	}
	a, err := s.payments.SaveSession(ctx, a.ID, session)
	if err != nil {
		return a, err
	}
	if a.Status != domain.PaymentPending || !session.Payable || !a.ReservationExpiresAt.After(time.Now()) || a.SessionExpiresAt == nil || !a.SessionExpiresAt.After(time.Now()) {
		return withoutURL(a), nil
	}
	return a, nil
}

func withoutURL(a domain.Payment) domain.Payment { a.PaymentURL = ""; return a }

// Session creation uses the same saved request and provider key when reconciling
// an uncertain creation. Callers must not expose its URL after stock expiry.
func (s *PaymentService) createSession(ctx context.Context, a domain.Payment) (domain.Payment, error) {
	expiresAt := a.CreatedAt.Add(time.Hour)
	// Avoid replaying after Stripe's key retention window or with an expiry that
	// can no longer satisfy its minimum session duration.
	if time.Since(a.CreatedAt) >= 23*time.Hour || expiresAt.Before(time.Now().Add(30*time.Minute)) || a.SuccessURL == "" || a.CancelURL == "" {
		return withoutURL(a), domain.ErrPaymentUnavailable
	}
	session, err := s.stripe.CreateCheckoutSession(ctx, port.CreateSessionRequest{
		PaymentID: a.ID, OrderID: a.OrderID, CustomerID: a.CustomerID, AmountUSDCents: a.AmountUSDCents,
		Currency: a.Currency, IdempotencyKey: a.ProviderIdempotencyKey, SuccessURL: a.SuccessURL, CancelURL: a.CancelURL, ExpiresAt: expiresAt,
	})
	return s.saveProviderSession(ctx, a, session, err)
}

func normalizePaymentRequest(cmd port.PreparePaymentCommand) (port.PreparePaymentCommand, error) {
	var err error
	cmd.OrderID, err = domain.NormalizeID("order_id", cmd.OrderID)
	if err != nil {
		return cmd, err
	}
	cmd.CustomerID, err = domain.NormalizeID("customer_id", cmd.CustomerID)
	if err != nil {
		return cmd, err
	}
	cmd.ReservationID, err = domain.NormalizeID("reservation_id", cmd.ReservationID)
	if err != nil {
		return cmd, err
	}
	if err := domain.ValidateAmount(cmd.AmountUSDCents, cmd.Currency); err != nil {
		return cmd, err
	}
	if cmd.ReservationExpiresAt.IsZero() {
		return cmd, &domain.ValidationError{Field: "reservation_expires_at", Message: "is required"}
	}

	return cmd, nil
}

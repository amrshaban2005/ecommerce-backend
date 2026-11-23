package port

import (
	"context"
	"time"

	"ecommerce.local/payment-service/internal/domain"
)

type StripeGateway interface {
	ExpireCheckoutSession(context.Context, string) (CheckoutSession, error)
	CreateCheckoutSession(context.Context, CreateSessionRequest) (CheckoutSession, error)
	GetCheckoutSession(context.Context, string) (CheckoutSession, error)
}

type WebhookVerifier interface {
	VerifyWebhook([]byte, string) (VerifiedWebhook, error)
}

type CreateSessionRequest struct {
	PaymentID      string
	OrderID        string
	CustomerID     string
	AmountUSDCents int64
	Currency       string
	IdempotencyKey string
	SuccessURL     string
	CancelURL      string
	ExpiresAt      time.Time
}

// Stripe-specific statuses stay in its adapter. Pending alone does not mean a
// session is payable: Payable must also be true before exposing its URL.
type CheckoutSession struct {
	SessionID       string
	PaymentIntentID string
	PaymentID       string
	OrderID         string
	CustomerID      string
	AmountUSDCents  int64
	Currency        string
	Status          domain.PaymentStatus
	Payable         bool
	PaymentURL      string
	ExpiresAt       *time.Time
	LiveMode        bool
}

type VerifiedWebhook struct {
	EventID   string
	EventType string
	SessionID string
	Supported bool
	LiveMode  bool
}

type StoredWebhook struct {
	EventID   string
	SessionID string
	Processed bool
}

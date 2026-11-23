package port

import (
	"context"
	"time"

	"ecommerce.local/payment-service/internal/domain"
)

type PaymentCheckout interface {
	ClosePayment(context.Context, PreparePaymentCommand) (domain.Payment, error)
	PreparePayment(context.Context, PreparePaymentCommand) (domain.Payment, error)
	GetOrderPayment(context.Context, string) (domain.Payment, error)
}

type PaymentWebhooks interface {
	ReceiveWebhook(context.Context, []byte, string) error
}
type PaymentRecovery interface {
	Recover(context.Context, int) error
}
type PaymentWorkflow interface {
	PaymentCheckout
	PaymentWebhooks
	PaymentRecovery
}

type PreparePaymentCommand struct {
	OrderID              string
	CustomerID           string
	ReservationID        string
	ReservationExpiresAt time.Time
	AmountUSDCents       int64
	Currency             string
}

type PaymentOutcome struct {
	OrderID        string
	PaymentID      string
	CustomerID     string
	AmountUSDCents int64
	Currency       string
	Status         domain.PaymentStatus
}

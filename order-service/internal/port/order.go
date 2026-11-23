package port

import (
	"context"
	"errors"
	"time"

	"ecommerce.local/order-service/internal/domain"
)

var ErrOrderNotFound = errors.New("order not found")

type CheckoutCommand struct {
	CustomerID     string
	IdempotencyKey string
}

// CheckoutResult is the application response; external payment data stays out of
// the Order domain model and is mapped by the HTTP adapter.
type CheckoutResult struct {
	Order   domain.Order
	Payment *PaymentSession
}

// HTTP and the recovery worker are inbound adapters to the same order workflow.
type Checkout interface {
	Start(context.Context, CheckoutCommand) (CheckoutResult, error)
}

type CheckoutRecovery interface {
	Recover(context.Context, int) error
}

type CheckoutWorkflow interface {
	Checkout
	CheckoutRecovery
	PaymentOutcomeRecorder
}

type OrderRepository interface {
	MarkCompleted(context.Context, string) (bool, error)
	MarkExpiryCleanup(ctx context.Context, orderID, paymentID, failureCode string) (bool, error)
	RecordPaymentOutcome(context.Context, PaymentOutcomeCommand) (bool, error)
	FindByKey(ctx context.Context, customerID, key string) (domain.Order, error)
	SavePending(ctx context.Context, order domain.Order, retryDelay time.Duration) (domain.Order, bool, error)
	LoadOrder(context.Context, string) (domain.Order, error)
	ListRecovery(context.Context, int) ([]string, error)
	CompleteCheckout(ctx context.Context, orderID, reservationID string, expiresAt time.Time) (bool, error)
	MarkCleanup(ctx context.Context, orderID, failureCode string) (bool, error)
	FailCheckout(context.Context, string) (bool, error)
	RetryCheckout(ctx context.Context, orderID, message string, delay time.Duration) error
}

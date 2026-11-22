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

// HTTP and the recovery worker are inbound adapters to the same order workflow.
type Checkout interface {
	Start(context.Context, CheckoutCommand) (domain.Order, error)
}

type CheckoutRecovery interface {
	Recover(context.Context, int) error
}

type CheckoutWorkflow interface {
	Checkout
	CheckoutRecovery
}

type OrderRepository interface {
	FindByKey(ctx context.Context, customerID, key string) (domain.Order, error)
	SavePending(ctx context.Context, order domain.Order, retryDelay time.Duration) (domain.Order, bool, error)
	LoadOrder(context.Context, string) (domain.Order, error)
	ListRecovery(context.Context, int) ([]string, error)
	CompleteCheckout(ctx context.Context, orderID, reservationID string, expiresAt time.Time) (bool, error)
	MarkCleanup(ctx context.Context, orderID, failureCode string) (bool, error)
	FailCheckout(context.Context, string) (bool, error)
	RetryCheckout(ctx context.Context, orderID, message string, delay time.Duration) error
}

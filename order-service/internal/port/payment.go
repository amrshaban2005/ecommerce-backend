package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrPaymentUnavailable = errors.New("payment unavailable")
)

// PaymentPreparer is Order's outbound payment dependency. Integration data stays
// here; provider SDK types and protobufs belong to adapters.
type PaymentPreparer interface {
	ClosePayment(context.Context, PaymentRequest) (PaymentSession, error)
	PreparePayment(context.Context, PaymentRequest) (PaymentSession, error)
	GetOrderPayment(context.Context, string) (PaymentSession, error)
}

type PaymentRequest struct {
	OrderID              string
	CustomerID           string
	ReservationID        string
	ReservationExpiresAt time.Time
	AmountUSDCents       int64
	Currency             string
}

type PaymentSession struct {
	PaymentID        string
	OrderID          string
	CustomerID       string
	AmountUSDCents   int64
	Currency         string
	Status           string
	PaymentURL       string
	SessionExpiresAt *time.Time
}

// PaymentOutcomeRecorder is inbound, used by transport adapters to record payment results.
// Order checks ownership and the saved amount; PAID cannot be overwritten.
type PaymentOutcomeRecorder interface {
	RecordPaymentOutcome(context.Context, PaymentOutcomeCommand) error
}

type PaymentOutcomeCommand struct {
	OrderID        string
	PaymentID      string
	CustomerID     string
	AmountUSDCents int64
	Currency       string
	Status         string
}

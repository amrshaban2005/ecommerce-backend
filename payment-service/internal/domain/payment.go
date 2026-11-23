package domain

import "time"

// A timeout or processing result stays PENDING. FAILED means a confirmed
// closed and unpaid session; a card decline in a usable session stays PENDING.
type PaymentStatus string

const (
	PaymentPending PaymentStatus = "PENDING"
	PaymentPaid    PaymentStatus = "PAID"
	PaymentFailed  PaymentStatus = "FAILED"
)

// Payment belongs to Payment. Order/customer/reservation IDs reference
// other services; Payment never reads or writes their databases.
type Payment struct {
	EventPublished          bool
	CreatedAt               time.Time
	SuccessURL              string
	CancelURL               string
	ID                      string
	OrderID                 string
	CustomerID              string
	ReservationID           string
	ReservationExpiresAt    time.Time
	AmountUSDCents          int64
	Currency                string
	Status                  PaymentStatus
	ProviderIdempotencyKey  string
	ProviderSessionID       string
	ProviderPaymentIntentID string
	PaymentURL              string
	SessionExpiresAt        *time.Time
}

package events

const (
	PaymentExchange   = "payments"
	OrderPaymentQueue = "order.payments"
	PaymentPaid       = "payment.paid"
	PaymentFailed     = "payment.failed"
)

// PaymentOutcome is the JSON contract shared by the messaging adapters.
// EventID is stable for a payment and outcome, including publication retries.
type PaymentOutcome struct {
	EventID        string `json:"event_id"`
	Type           string `json:"type"`
	OrderID        string `json:"order_id"`
	PaymentID      string `json:"payment_id"`
	CustomerID     string `json:"customer_id"`
	AmountUSDCents int64  `json:"amount_usd_cents"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
}

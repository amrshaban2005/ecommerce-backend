package port

import "context"

// PaymentPublisher is implemented by the messaging adapter. Success means the
// broker confirmed receipt; only then may the saved outcome be marked delivered.
type PaymentPublisher interface {
	PublishPaymentOutcome(context.Context, PaymentOutcome) error
}

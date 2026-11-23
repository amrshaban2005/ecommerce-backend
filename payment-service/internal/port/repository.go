package port

import (
	"context"
	"ecommerce.local/payment-service/internal/domain"
)

type PaymentRepository interface {
	FindByOrder(context.Context, string) (domain.Payment, error)
	FindByID(context.Context, string) (domain.Payment, error)
	CreateOrGet(context.Context, domain.Payment) (domain.Payment, error)
	SaveSession(context.Context, string, CheckoutSession) (domain.Payment, error)
	PendingEvents(context.Context, int) ([]domain.Payment, error)
	MarkPublished(context.Context, string, domain.PaymentStatus) error
	RetryPublish(context.Context, string) error
}

type WebhookRepository interface {
	Save(context.Context, VerifiedWebhook, []byte) (StoredWebhook, error)
	MarkProcessed(ctx context.Context, eventID, paymentID string) error
	MarkIgnored(context.Context, string) error
	Pending(context.Context, int) ([]StoredWebhook, error)
	Retry(context.Context, string) error
}

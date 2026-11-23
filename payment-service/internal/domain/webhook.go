package domain

import "time"

type WebhookStatus string

const (
	WebhookPending   WebhookStatus = "PENDING"
	WebhookProcessed WebhookStatus = "PROCESSED"
	WebhookIgnored   WebhookStatus = "IGNORED"
)

// WebhookEvent identifies a verified provider event. Its external event ID is
// retained for deduplication; it is not an application-generated UUID.
type WebhookEvent struct {
	EventID     string
	EventType   string
	PaymentID   string
	Status      WebhookStatus
	ReceivedAt  time.Time
	ProcessedAt *time.Time
}

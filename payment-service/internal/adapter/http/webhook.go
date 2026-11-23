package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
)

type WebhookHandler struct{ payments port.PaymentWebhooks }

func NewWebhookHandler(payments port.PaymentWebhooks) *WebhookHandler {
	return &WebhookHandler{payments: payments}
}
func (h *WebhookHandler) Receive(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			webhookError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "webhook payload too large")
		} else {
			webhookError(w, http.StatusBadRequest, "INVALID_WEBHOOK", "cannot read webhook")
		}
		return
	}
	signatures := r.Header.Values("Stripe-Signature")
	if len(signatures) != 1 {
		webhookError(w, http.StatusBadRequest, "INVALID_WEBHOOK", "Stripe signature required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := h.payments.ReceiveWebhook(ctx, payload, signatures[0]); err != nil {
		if errors.Is(err, domain.ErrInvalidWebhook) {
			webhookError(w, http.StatusBadRequest, "INVALID_WEBHOOK", "invalid Stripe webhook")
		} else {
			slog.Warn("webhook processing deferred", "error", err, "service", "payment-service")
			webhookError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "webhook processing temporarily unavailable")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]bool{"received": true}, "errors": []any{}})
}
func webhookError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": nil, "errors": []map[string]string{{"error_code": code, "message": message}}})
}

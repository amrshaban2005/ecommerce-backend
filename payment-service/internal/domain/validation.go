package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidWebhook     = errors.New("invalid Stripe webhook")
	ErrPaymentNotFound    = errors.New("payment attempt not found")
	ErrPaymentMismatch    = errors.New("payment request does not match the saved order")
	ErrReservationExpired = errors.New("stock reservation is expired")
	ErrPaymentUnavailable = errors.New("payment session is unavailable")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func NormalizeID(field, value string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return "", &ValidationError{Field: field, Message: "must be a valid non-zero UUID"}
	}
	return id.String(), nil
}

func ValidateAmount(cents int64, currency string) error {
	if currency != "USD" {
		return &ValidationError{Field: "currency", Message: "must be USD"}
	}
	if cents < 50 || cents > 99999999 {
		return &ValidationError{Field: "amount_usd_cents", Message: "must be between 50 and 99999999 USD cents"}
	}
	return nil
}

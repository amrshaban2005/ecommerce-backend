package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ReservationActive   = "ACTIVE"
	ReservationReleased = "RELEASED"
)

var (
	ErrInsufficientStock      = errors.New("insufficient stock")
	ErrReservationNotFound    = errors.New("reservation not found")
	ErrReservationConflict    = errors.New("order already has a reservation with different items")
	ErrReservationUnavailable = errors.New("reservation is released or expired")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

type ReservationItem struct {
	ProductID string
	Quantity  int32
}

type Reservation struct {
	ID        string
	OrderID   string
	Status    string
	ExpiresAt time.Time
	Items     []ReservationItem
}

func NormalizeOrderID(value string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return "", &ValidationError{Field: "order_id", Message: "must be a valid non-zero UUID"}
	}
	return id.String(), nil
}

func NormalizeItems(items []ReservationItem) ([]ReservationItem, error) {
	if len(items) == 0 || len(items) > 1000 {
		return nil, &ValidationError{Field: "items", Message: "must contain between 1 and 1000 items"}
	}
	result := make([]ReservationItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		id, err := uuid.Parse(strings.TrimSpace(item.ProductID))
		if err != nil || id == uuid.Nil {
			return nil, &ValidationError{Field: "product_id", Message: "must be a valid non-zero UUID"}
		}
		item.ProductID = id.String()
		if item.Quantity <= 0 {
			return nil, &ValidationError{Field: "quantity", Message: "must be a positive integer"}
		}
		if seen[item.ProductID] {
			return nil, &ValidationError{Field: "items", Message: "must not contain duplicate products"}
		}
		seen[item.ProductID] = true
		result = append(result, item)
	}
	// Every transaction updates stock in the same order to avoid lock cycles.
	sort.Slice(result, func(i, j int) bool { return result[i].ProductID < result[j].ProductID })
	return result, nil
}

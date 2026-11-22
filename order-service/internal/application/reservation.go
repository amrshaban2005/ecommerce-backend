package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"github.com/google/uuid"
)

func (s *CheckoutService) reserveOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if order.Status != domain.OrderPending {
		return order, nil
	}
	items := make([]port.CartItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, port.CartItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}

	// A timed-out request may already have reserved stock. Reuse that reservation.
	reservation, err := s.inventory.GetReservation(ctx, order.ID)
	if errors.Is(err, port.ErrReservationNotFound) {
		reservation, err = s.inventory.ReserveStock(ctx, order.ID, items)
	}
	failureCode := ""
	switch {
	case errors.Is(err, port.ErrInsufficientStock):
		failureCode = "INSUFFICIENT_STOCK"
	case errors.Is(err, port.ErrReservationConflict), errors.Is(err, port.ErrReservationUnavailable):
		failureCode = "RESERVATION_UNAVAILABLE"
	case err != nil:
		return s.scheduleRetry(ctx, order, fmt.Errorf("reserve order %s: %w", order.ID, err))
	case reservation.Status == "RELEASED" || !reservation.ExpiresAt.After(time.Now()):
		failureCode = "RESERVATION_UNAVAILABLE"
	default:
		if err := validateReservation(order.ID, items, reservation); err != nil {
			failureCode = "CHECKOUT_FAILED"
		}
	}
	if failureCode != "" {
		order.FailureCode = failureCode
		return s.releaseOrder(ctx, order)
	}

	completed, err := s.orders.CompleteCheckout(ctx, order.ID, reservation.ID, reservation.ExpiresAt)
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	if !completed {
		return s.orders.LoadOrder(ctx, order.ID)
	}
	order.Status = domain.OrderAwaitingPayment
	order.ReservationID = reservation.ID
	order.ReservationExpiresAt = &reservation.ExpiresAt
	return order, nil
}

func (s *CheckoutService) releaseOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if order.Status == domain.OrderPending {
		// Cleanup and completion compete for the same PENDING state. Never release
		// inventory if completion has already made the order ready for payment.
		marked, err := s.orders.MarkCleanup(ctx, order.ID, order.FailureCode)
		if err != nil {
			return s.scheduleRetry(ctx, order, err)
		}
		if !marked {
			return s.orders.LoadOrder(ctx, order.ID)
		}
		order.Status = domain.OrderCleanupPending
	}
	if order.Status != domain.OrderCleanupPending {
		return order, nil
	}
	reservation, err := s.inventory.ReleaseReservation(ctx, order.ID)
	if err != nil {
		return s.scheduleRetry(ctx, order, fmt.Errorf("release order %s: %w", order.ID, err))
	}
	if reservation.OrderID != order.ID || reservation.Status != "RELEASED" {
		return s.scheduleRetry(ctx, order, invalidDependency("Inventory", "reservation release was not confirmed"))
	}
	failed, err := s.orders.FailCheckout(ctx, order.ID)
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	if !failed {
		return s.orders.LoadOrder(ctx, order.ID)
	}
	order.Status = domain.OrderCheckoutFailed
	return order, nil
}

func invalidDependency(service, message string) error {
	return &port.DependencyError{Service: service, Err: errors.New(message)}
}

func validateReservation(orderID string, cart []port.CartItem, reservation port.Reservation) error {
	id, err := uuid.Parse(reservation.ID)
	if err != nil || id == uuid.Nil || reservation.OrderID != orderID || reservation.Status != "ACTIVE" || !reservation.ExpiresAt.After(time.Now()) {
		return invalidDependency("Inventory", "invalid or expired checkout reservation")
	}
	expected := slices.Clone(cart)
	actual := slices.Clone(reservation.Items)
	sort.Slice(expected, func(i, j int) bool { return expected[i].ProductID < expected[j].ProductID })
	sort.Slice(actual, func(i, j int) bool { return actual[i].ProductID < actual[j].ProductID })
	if !slices.Equal(expected, actual) {
		return invalidDependency("Inventory", "reservation items do not match the order")
	}
	return nil
}

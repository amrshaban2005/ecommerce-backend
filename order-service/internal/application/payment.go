package application

import (
	"context"
	"errors"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"github.com/google/uuid"
)

func (s *CheckoutService) preparePayment(ctx context.Context, order domain.Order) (port.CheckoutResult, error) {
	result := port.CheckoutResult{Order: order}
	if order.Status != domain.OrderAwaitingPayment {
		return result, nil
	}
	cents, err := domain.AmountUSDCents(order.TotalUSD)
	if err != nil {
		return result, err
	}
	// Check the existing payment first, including a paid outcome that
	// arrived near reservation expiry. Never start a second session in that case.
	payment, err := s.payments.GetOrderPayment(ctx, order.ID)
	exists := err == nil
	if errors.Is(err, port.ErrPaymentUnavailable) {
		return result, domain.ErrPaymentUnavailable
	}
	if err != nil && !errors.Is(err, port.ErrPaymentNotFound) {
		return result, err
	}
	if exists {
		if err := validatePayment(order, cents, payment); err != nil {
			return result, err
		}
		switch payment.Status {
		case "PAID":
			payment.PaymentURL = ""
			result.Payment = &payment
			return result, nil
		case "FAILED":
			return result, domain.ErrPaymentUnavailable
		}
	}
	reservation, err := s.inventory.GetReservation(ctx, order.ID)
	if errors.Is(err, port.ErrReservationNotFound) {
		return result, domain.ErrReservationUnavailable
	}
	if err != nil {
		return result, err
	}
	items := make([]port.CartItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, port.CartItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	if order.ReservationExpiresAt == nil || reservation.ID != order.ReservationID || !reservation.ExpiresAt.Equal(*order.ReservationExpiresAt) || validateReservation(order.ID, items, reservation) != nil {
		return result, domain.ErrReservationUnavailable
	}
	if !exists || payment.PaymentURL == "" {
		payment, err = s.payments.PreparePayment(ctx, port.PaymentRequest{
			OrderID: order.ID, CustomerID: order.CustomerID, ReservationID: order.ReservationID,
			ReservationExpiresAt: *order.ReservationExpiresAt, AmountUSDCents: cents, Currency: order.Currency,
		})
		if errors.Is(err, port.ErrPaymentUnavailable) {
			return result, domain.ErrPaymentUnavailable
		}
		if err != nil {
			return result, err
		}
		if err := validatePayment(order, cents, payment); err != nil {
			return result, err
		}
	}
	if payment.Status == "FAILED" {
		return result, domain.ErrPaymentUnavailable
	}
	if payment.PaymentURL != "" && (payment.SessionExpiresAt == nil || !payment.SessionExpiresAt.After(time.Now()) || !order.ReservationExpiresAt.After(time.Now())) {
		return result, domain.ErrReservationUnavailable
	}
	result.Payment = &payment
	return result, nil
}

func validatePayment(order domain.Order, cents int64, payment port.PaymentSession) error {
	id, err := uuid.Parse(payment.PaymentID)
	if err != nil || id == uuid.Nil || payment.OrderID != order.ID || payment.CustomerID != order.CustomerID || payment.AmountUSDCents != cents || payment.Currency != order.Currency {
		return invalidDependency("Payment", "payment does not match the saved order")
	}
	return nil
}

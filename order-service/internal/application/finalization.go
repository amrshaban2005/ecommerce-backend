package application

import (
	"context"
	"fmt"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
)

func (s *CheckoutService) finalizeOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if order.Status != domain.OrderPaid {
		return order, nil
	}
	if order.PaymentStatus != "PAID" {
		return s.scheduleRetry(ctx, order, fmt.Errorf("paid payment confirmation is required"))
	}
	reservation, err := s.inventory.FinalizeReservation(ctx, order.ID)
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	if reservation.OrderID != order.ID || reservation.ID != order.ReservationID || reservation.Status != "FINALIZED" {
		return s.scheduleRetry(ctx, order, invalidDependency("Inventory", "stock finalization was not confirmed"))
	}
	items := make([]port.CartItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, port.CartItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	if err := s.carts.RemovePurchasedItems(ctx, order.ID, order.CustomerID, items); err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	completed, err := s.orders.MarkCompleted(ctx, order.ID)
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	if !completed {
		return s.orders.LoadOrder(ctx, order.ID)
	}
	order.Status = domain.OrderCompleted
	return order, nil
}

func (s *CheckoutService) expireOrder(ctx context.Context, order domain.Order) (domain.Order, error) {
	if order.Status != domain.OrderAwaitingPayment && order.Status != domain.OrderCleanupPending {
		return order, nil
	}
	if order.ReservationExpiresAt == nil {
		return s.scheduleRetry(ctx, order, fmt.Errorf("saved reservation expiry is missing"))
	}
	cents, err := domain.AmountUSDCents(order.TotalUSD)
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	// Close first, even for a recorded failure: the provider may have an in-flight
	// outcome. PENDING or an error always keeps the stock reserved.
	payment, err := s.payments.ClosePayment(ctx, port.PaymentRequest{OrderID: order.ID, CustomerID: order.CustomerID,
		ReservationID: order.ReservationID, ReservationExpiresAt: *order.ReservationExpiresAt, AmountUSDCents: cents, Currency: order.Currency})
	if err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	if err := validatePayment(order, cents, payment); err != nil {
		return s.scheduleRetry(ctx, order, err)
	}
	switch payment.Status {
	case "PAID":
		err := s.RecordPaymentOutcome(ctx, port.PaymentOutcomeCommand{OrderID: order.ID, PaymentID: payment.PaymentID, CustomerID: order.CustomerID,
			AmountUSDCents: cents, Currency: order.Currency, Status: "PAID"})
		if err != nil {
			return s.scheduleRetry(ctx, order, err)
		}
		return s.orders.LoadOrder(ctx, order.ID)
	case "FAILED":
		if order.Status == domain.OrderAwaitingPayment {
			code := "PAYMENT_FAILED"
			if !order.ReservationExpiresAt.After(time.Now()) {
				code = "RESERVATION_EXPIRED"
			}
			marked, err := s.orders.MarkExpiryCleanup(ctx, order.ID, payment.PaymentID, code)
			if err != nil {
				return s.scheduleRetry(ctx, order, err)
			}
			if !marked {
				return s.orders.LoadOrder(ctx, order.ID)
			}
			order.Status = domain.OrderCleanupPending
			order.PaymentID = payment.PaymentID
			order.PaymentStatus = "FAILED"
			order.FailureCode = code
		}
		return s.releaseOrder(ctx, order)
	default:
		return s.scheduleRetry(ctx, order, fmt.Errorf("payment is still pending; stock remains reserved"))
	}
}

package application

import (
	"context"
	"errors"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
)

type CheckoutService struct {
	orders            port.OrderRepository
	carts             port.CartGateway
	products          port.ProductReader
	inventory         port.InventoryReservations
	payments          port.PaymentPreparer
	processingTimeout time.Duration
}

func NewCheckoutService(orders port.OrderRepository, carts port.CartGateway, products port.ProductReader, inventory port.InventoryReservations, payments port.PaymentPreparer, timeout time.Duration) port.CheckoutWorkflow {
	return &CheckoutService{orders: orders, carts: carts, products: products, inventory: inventory, payments: payments, processingTimeout: timeout}
}

func (s *CheckoutService) Start(ctx context.Context, cmd port.CheckoutCommand) (port.CheckoutResult, error) {
	customerID, err := domain.NormalizeCustomerID(cmd.CustomerID)
	if err != nil {
		return port.CheckoutResult{}, err
	}
	if err := domain.ValidateIdempotencyKey(cmd.IdempotencyKey); err != nil {
		return port.CheckoutResult{}, err
	}
	cmd.CustomerID = customerID

	order, err := s.orders.FindByKey(ctx, cmd.CustomerID, cmd.IdempotencyKey)
	if err == nil {
		return s.preparePayment(ctx, order)
	}
	if !errors.Is(err, port.ErrOrderNotFound) {
		return port.CheckoutResult{}, err
	}

	order, created, err := s.createOrderFromCart(ctx, cmd)
	if err != nil {
		// A concurrent request may have saved this key while we were validating.
		if existing, findErr := s.orders.FindByKey(ctx, cmd.CustomerID, cmd.IdempotencyKey); findErr == nil {
			return s.preparePayment(ctx, existing)
		}
		return port.CheckoutResult{}, err
	}
	if !created {
		return s.preparePayment(ctx, order)
	}
	order, err = s.reserveOrder(ctx, order)
	if err != nil {
		return port.CheckoutResult{Order: order}, err
	}
	return s.preparePayment(ctx, order)
}

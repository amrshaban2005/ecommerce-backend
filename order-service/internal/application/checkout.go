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
	carts             port.CartReader
	products          port.ProductReader
	inventory         port.InventoryReservations
	processingTimeout time.Duration
}

func NewCheckoutService(orders port.OrderRepository, carts port.CartReader, products port.ProductReader, inventory port.InventoryReservations, timeout time.Duration) port.CheckoutWorkflow {
	return &CheckoutService{orders: orders, carts: carts, products: products, inventory: inventory, processingTimeout: timeout}
}

func (s *CheckoutService) Start(ctx context.Context, cmd port.CheckoutCommand) (domain.Order, error) {
	customerID, err := domain.NormalizeCustomerID(cmd.CustomerID)
	if err != nil {
		return domain.Order{}, err
	}
	if err := domain.ValidateIdempotencyKey(cmd.IdempotencyKey); err != nil {
		return domain.Order{}, err
	}
	cmd.CustomerID = customerID

	order, err := s.orders.FindByKey(ctx, cmd.CustomerID, cmd.IdempotencyKey)
	if err == nil {
		return order, nil
	}
	if !errors.Is(err, port.ErrOrderNotFound) {
		return domain.Order{}, err
	}

	order, created, err := s.createOrderFromCart(ctx, cmd)
	if err != nil {
		// A concurrent request may have saved this key while we were validating.
		if existing, findErr := s.orders.FindByKey(ctx, cmd.CustomerID, cmd.IdempotencyKey); findErr == nil {
			return existing, nil
		}
		return domain.Order{}, err
	}
	if !created {
		return order, nil
	}
	return s.reserveOrder(ctx, order)
}

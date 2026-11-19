package application

import (
	"context"
	"fmt"

	"ecommerce.local/cart-service/internal/domain"
	"ecommerce.local/cart-service/internal/port"
	"github.com/google/uuid"
)

type CartService struct{ carts port.CartRepository }

func NewCartService(carts port.CartRepository) port.Cart {
	return &CartService{carts: carts}
}

func (s *CartService) AddItem(ctx context.Context, cmd port.AddCartItemCommand) (domain.CartItem, error) {
	customerID, productID, err := domain.NormalizeCartItem(cmd.CustomerID, cmd.ProductID, cmd.Quantity)
	if err != nil {
		return domain.CartItem{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.CartItem{}, fmt.Errorf("create cart ID: %w", err)
	}
	// Product eligibility checks are deferred. Adding does not reserve inventory.
	return s.carts.AddItem(ctx, id.String(), customerID, productID, int32(cmd.Quantity))
}

func (s *CartService) View(ctx context.Context, customerID string) ([]domain.CartItem, error) {
	customerID, err := domain.NormalizeCustomerID(customerID)
	if err != nil {
		return nil, err
	}
	return s.carts.ListByCustomer(ctx, customerID)
}

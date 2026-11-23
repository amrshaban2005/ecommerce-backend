package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

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

func (s *CartService) RemovePurchasedItems(ctx context.Context, cmd port.RemovePurchasedCommand) error {
	orderID, err := uuid.Parse(cmd.OrderID)
	if err != nil || orderID == uuid.Nil {
		return &domain.ValidationError{Field: "order_id", Message: "must be a valid non-zero UUID"}
	}
	cmd.OrderID = orderID.String()
	cmd.CustomerID, err = domain.NormalizeCustomerID(cmd.CustomerID)
	if err != nil {
		return err
	}
	if len(cmd.Items) == 0 || len(cmd.Items) > 1000 {
		return &domain.ValidationError{Field: "items", Message: "must contain 1 to 1000 items"}
	}
	seen := make(map[string]bool, len(cmd.Items))
	for i := range cmd.Items {
		_, productID, err := domain.NormalizeCartItem(cmd.CustomerID, cmd.Items[i].ProductID, int64(cmd.Items[i].Quantity))
		if err != nil {
			return err
		}
		if seen[productID] {
			return &domain.ValidationError{Field: "items", Message: "must not contain duplicate products"}
		}
		seen[productID] = true
		cmd.Items[i].ProductID = productID
	}
	sort.Slice(cmd.Items, func(i, j int) bool { return cmd.Items[i].ProductID < cmd.Items[j].ProductID })
	payload, err := json.Marshal(cmd.Items)
	if err != nil {
		return err
	}
	return s.carts.RemovePurchasedItems(ctx, cmd, fmt.Sprintf("%x", sha256.Sum256(payload)))
}

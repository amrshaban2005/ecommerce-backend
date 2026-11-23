package application

import (
	"context"
	"fmt"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"github.com/google/uuid"
)

func (s *CheckoutService) createOrderFromCart(ctx context.Context, cmd port.CheckoutCommand) (domain.Order, bool, error) {
	cart, err := s.carts.GetCustomerCart(ctx, cmd.CustomerID)
	if err != nil {
		return domain.Order{}, false, err
	}
	if len(cart) == 0 {
		return domain.Order{}, false, domain.ErrEmptyCart
	}
	if len(cart) > 1000 {
		return domain.Order{}, false, invalidDependency("Cart", "cart exceeds the supported item limit")
	}
	ids := make([]string, 0, len(cart))
	seen := make(map[string]bool, len(cart))
	for i := range cart {
		id, err := uuid.Parse(cart[i].ProductID)
		if err != nil || id == uuid.Nil || cart[i].Quantity <= 0 {
			return domain.Order{}, false, invalidDependency("Cart", "invalid cart item")
		}
		cart[i].ProductID = id.String()
		if seen[cart[i].ProductID] {
			return domain.Order{}, false, invalidDependency("Cart", "duplicate cart product")
		}
		seen[cart[i].ProductID] = true
		ids = append(ids, cart[i].ProductID)
	}
	products, err := s.products.GetCheckoutProducts(ctx, ids)
	if err != nil {
		return domain.Order{}, false, err
	}
	byID := make(map[string]port.CheckoutProduct, len(products))
	for _, product := range products {
		if !seen[product.ProductID] {
			return domain.Order{}, false, invalidDependency("Product", "unexpected checkout product")
		}
		if _, exists := byID[product.ProductID]; exists {
			return domain.Order{}, false, invalidDependency("Product", "duplicate checkout product")
		}
		byID[product.ProductID] = product
	}
	items := make([]domain.OrderItem, 0, len(cart))
	for _, item := range cart {
		product, exists := byID[item.ProductID]
		if !exists || !product.Active {
			return domain.Order{}, false, domain.ErrProductUnavailable
		}
		items = append(items, domain.OrderItem{
			ProductID: item.ProductID, ProductName: product.Name, Quantity: item.Quantity, UnitPriceUSD: product.PriceUSD,
		})
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("create order ID: %w", err)
	}
	order, err := domain.NewPendingOrder(id.String(), cmd.CustomerID, items)
	if err != nil {
		return domain.Order{}, false, &port.DependencyError{Service: "Product", Err: err}
	}
	if _, err := domain.AmountUSDCents(order.TotalUSD); err != nil {
		return domain.Order{}, false, err
	}
	order.IdempotencyKey = cmd.IdempotencyKey
	return s.orders.SavePending(ctx, order, s.processingTimeout+5*time.Second)
}

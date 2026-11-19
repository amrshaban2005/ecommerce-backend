package port

import (
	"context"

	"ecommerce.local/cart-service/internal/domain"
)

type AddCartItemCommand struct {
	CustomerID string
	ProductID  string
	Quantity   int64
}

type Cart interface {
	View(context.Context, string) ([]domain.CartItem, error)
	AddItem(context.Context, AddCartItemCommand) (domain.CartItem, error)
}

type CartRepository interface {
	ListByCustomer(context.Context, string) ([]domain.CartItem, error)
	AddItem(ctx context.Context, cartID, customerID, productID string, quantity int32) (domain.CartItem, error)
}

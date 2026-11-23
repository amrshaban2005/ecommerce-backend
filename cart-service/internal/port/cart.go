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
	RemovePurchasedItems(context.Context, RemovePurchasedCommand) error
	View(context.Context, string) ([]domain.CartItem, error)
	AddItem(context.Context, AddCartItemCommand) (domain.CartItem, error)
}

type CartRepository interface {
	RemovePurchasedItems(context.Context, RemovePurchasedCommand, string) error
	ListByCustomer(context.Context, string) ([]domain.CartItem, error)
	AddItem(ctx context.Context, cartID, customerID, productID string, quantity int32) (domain.CartItem, error)
}

type PurchasedItem struct {
	ProductID string
	Quantity  int32
}
type RemovePurchasedCommand struct {
	OrderID    string
	CustomerID string
	Items      []PurchasedItem
}

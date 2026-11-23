package port

import "context"

type CartCleaner interface {
	RemovePurchasedItems(ctx context.Context, orderID, customerID string, items []CartItem) error
}
type CartGateway interface {
	CartReader
	CartCleaner
}

// Order-owned outbound ports isolate checkout from service transports.
type CartReader interface {
	GetCustomerCart(context.Context, string) ([]CartItem, error)
}

type ProductReader interface {
	GetCheckoutProducts(context.Context, []string) ([]CheckoutProduct, error)
}

type InventoryReservations interface {
	FinalizeReservation(context.Context, string) (Reservation, error)
	ReserveStock(context.Context, string, []CartItem) (Reservation, error)
	GetReservation(context.Context, string) (Reservation, error)
	ReleaseReservation(context.Context, string) (Reservation, error)
}

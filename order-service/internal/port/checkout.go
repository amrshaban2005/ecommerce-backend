package port

import "context"

// Order-owned outbound ports isolate checkout from service transports.
type CartReader interface {
	GetCustomerCart(context.Context, string) ([]CartItem, error)
}

type ProductReader interface {
	GetCheckoutProducts(context.Context, []string) ([]CheckoutProduct, error)
}

type InventoryReservations interface {
	ReserveStock(context.Context, string, []CartItem) (Reservation, error)
	GetReservation(context.Context, string) (Reservation, error)
	ReleaseReservation(context.Context, string) (Reservation, error)
}

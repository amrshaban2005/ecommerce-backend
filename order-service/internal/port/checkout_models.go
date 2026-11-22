package port

import "time"

// CartItem is the checkout input returned by the CartReader port.
type CartItem struct {
	ProductID string
	Quantity  int32
}

// CheckoutProduct is the authoritative catalog data required by Order.
type CheckoutProduct struct {
	ProductID string
	Name      string
	Active    bool
	PriceUSD  string
}

// Reservation is the inventory result consumed by checkout orchestration.
type Reservation struct {
	ID        string
	OrderID   string
	Status    string
	ExpiresAt time.Time
	Items     []CartItem
}

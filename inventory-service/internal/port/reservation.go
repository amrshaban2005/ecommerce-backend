package port

import (
	"context"
	"ecommerce.local/inventory-service/internal/domain"
)

type ReserveStockCommand struct {
	OrderID string
	Items   []domain.ReservationItem
}

type Inventory interface {
	FinalizeReservation(context.Context, string) (domain.Reservation, error)
	ReserveStock(context.Context, ReserveStockCommand) (domain.Reservation, error)
	GetReservation(context.Context, string) (domain.Reservation, error)
	ReleaseReservation(context.Context, string) (domain.Reservation, error)
}

type ReservationRepository interface {
	FinalizeByOrder(context.Context, string) (domain.Reservation, error)
	Reserve(ctx context.Context, reservationID, orderID string, items []domain.ReservationItem) (domain.Reservation, error)
	GetByOrder(context.Context, string) (domain.Reservation, error)
	ReleaseByOrder(ctx context.Context, reservationID, orderID string) (domain.Reservation, error)
}

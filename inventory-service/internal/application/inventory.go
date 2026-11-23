package application

import (
	"context"
	"fmt"

	"ecommerce.local/inventory-service/internal/domain"
	"ecommerce.local/inventory-service/internal/port"
	"github.com/google/uuid"
)

type InventoryService struct{ reservations port.ReservationRepository }

func NewInventoryService(reservations port.ReservationRepository) port.Inventory {
	return &InventoryService{reservations: reservations}
}

func (s *InventoryService) ReserveStock(ctx context.Context, cmd port.ReserveStockCommand) (domain.Reservation, error) {
	orderID, err := domain.NormalizeOrderID(cmd.OrderID)
	if err != nil {
		return domain.Reservation{}, err
	}
	items, err := domain.NormalizeItems(cmd.Items)
	if err != nil {
		return domain.Reservation{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("create reservation ID: %w", err)
	}
	return s.reservations.Reserve(ctx, id.String(), orderID, items)
}

func (s *InventoryService) GetReservation(ctx context.Context, orderID string) (domain.Reservation, error) {
	orderID, err := domain.NormalizeOrderID(orderID)
	if err != nil {
		return domain.Reservation{}, err
	}
	return s.reservations.GetByOrder(ctx, orderID)
}

func (s *InventoryService) ReleaseReservation(ctx context.Context, orderID string) (domain.Reservation, error) {
	orderID, err := domain.NormalizeOrderID(orderID)
	if err != nil {
		return domain.Reservation{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("create cancellation ID: %w", err)
	}
	return s.reservations.ReleaseByOrder(ctx, id.String(), orderID)
}

func (s *InventoryService) FinalizeReservation(ctx context.Context, orderID string) (domain.Reservation, error) {
	orderID, err := domain.NormalizeOrderID(orderID)
	if err != nil {
		return domain.Reservation{}, err
	}
	return s.reservations.FinalizeByOrder(ctx, orderID)
}

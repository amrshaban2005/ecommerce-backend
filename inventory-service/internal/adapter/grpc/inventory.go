package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	inventoryv1 "ecommerce.local/contracts/inventory/v1"
	"ecommerce.local/inventory-service/internal/domain"
	"ecommerce.local/inventory-service/internal/port"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type InventoryServer struct {
	inventoryv1.UnimplementedInventoryServiceServer
	inventory port.Inventory
}

func NewInventoryServer(inventory port.Inventory) *InventoryServer {
	return &InventoryServer{inventory: inventory}
}

func (s *InventoryServer) ReserveStock(ctx context.Context, req *inventoryv1.ReserveStockRequest) (*inventoryv1.Reservation, error) {
	items := make([]domain.ReservationItem, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, domain.ReservationItem{ProductID: item.GetProductId(), Quantity: item.GetQuantity()})
	}
	reservation, err := s.inventory.ReserveStock(ctx, port.ReserveStockCommand{OrderID: req.GetOrderId(), Items: items})
	return reservationResponse(reservation, err)
}

func (s *InventoryServer) GetReservation(ctx context.Context, req *inventoryv1.GetReservationRequest) (*inventoryv1.Reservation, error) {
	reservation, err := s.inventory.GetReservation(ctx, req.GetOrderId())
	return reservationResponse(reservation, err)
}

func (s *InventoryServer) ReleaseReservation(ctx context.Context, req *inventoryv1.ReleaseReservationRequest) (*inventoryv1.Reservation, error) {
	reservation, err := s.inventory.ReleaseReservation(ctx, req.GetOrderId())
	return reservationResponse(reservation, err)
}

func reservationResponse(reservation domain.Reservation, err error) (*inventoryv1.Reservation, error) {
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			return nil, status.Error(codes.InvalidArgument, validationError.Error())
		case errors.Is(err, domain.ErrInsufficientStock):
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		case errors.Is(err, domain.ErrReservationNotFound):
			return nil, status.Error(codes.NotFound, err.Error())
		case errors.Is(err, domain.ErrReservationConflict):
			return nil, status.Error(codes.AlreadyExists, err.Error())
		case errors.Is(err, domain.ErrReservationUnavailable):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "inventory deadline exceeded")
		default:
			slog.Error("inventory operation failed", "error", err, "service", "inventory-service")
			return nil, status.Error(codes.Internal, "inventory operation failed")
		}
	}
	state := inventoryv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	if reservation.Status == domain.ReservationReleased {
		state = inventoryv1.ReservationStatus_RESERVATION_STATUS_RELEASED
	}
	response := &inventoryv1.Reservation{
		ReservationId: reservation.ID, OrderId: reservation.OrderID, Status: state,
		ExpiresAt: timestamppb.New(reservation.ExpiresAt), Items: make([]*inventoryv1.ReservationItem, 0, len(reservation.Items)),
	}
	for _, item := range reservation.Items {
		response.Items = append(response.Items, &inventoryv1.ReservationItem{ProductId: item.ProductID, Quantity: item.Quantity})
	}
	return response, nil
}

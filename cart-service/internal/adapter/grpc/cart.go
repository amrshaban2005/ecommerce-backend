package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	"ecommerce.local/cart-service/internal/domain"
	"ecommerce.local/cart-service/internal/port"
	cartv1 "ecommerce.local/contracts/cart/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CartServer struct {
	cartv1.UnimplementedCartServiceServer
	cart port.Cart
}

func NewCartServer(cart port.Cart) *CartServer { return &CartServer{cart: cart} }

func (s *CartServer) GetCustomerCart(ctx context.Context, req *cartv1.GetCustomerCartRequest) (*cartv1.GetCustomerCartResponse, error) {
	items, err := s.cart.View(ctx, req.GetCustomerId())
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			return nil, status.Error(codes.InvalidArgument, validationError.Error())
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "cart lookup deadline exceeded")
		default:
			slog.Error("checkout cart lookup failed", "error", err, "service", "cart-service")
			return nil, status.Error(codes.Internal, "cart lookup failed")
		}
	}
	response := &cartv1.GetCustomerCartResponse{Items: make([]*cartv1.CartItem, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, &cartv1.CartItem{ProductId: item.ProductID, Quantity: item.Quantity})
	}
	return response, nil
}

func (s *CartServer) RemovePurchasedItems(ctx context.Context, req *cartv1.RemovePurchasedItemsRequest) (*cartv1.RemovePurchasedItemsResponse, error) {
	items := make([]port.PurchasedItem, 0, len(req.GetItems()))
	for _, item := range req.GetItems() {
		items = append(items, port.PurchasedItem{ProductID: item.GetProductId(), Quantity: item.GetQuantity()})
	}
	err := s.cart.RemovePurchasedItems(ctx, port.RemovePurchasedCommand{OrderID: req.GetOrderId(), CustomerID: req.GetCustomerId(), Items: items})
	if err != nil {
		var invalid *domain.ValidationError
		switch {
		case errors.As(err, &invalid):
			return nil, status.Error(codes.InvalidArgument, invalid.Error())
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "cart deadline exceeded")
		default:
			slog.Error("remove purchased items failed", "error", err, "service", "cart-service")
			return nil, status.Error(codes.Unavailable, "cart cleanup unavailable")
		}
	}
	return &cartv1.RemovePurchasedItemsResponse{}, nil
}

package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	orderv1 "ecommerce.local/contracts/order/v1"
	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
	orders port.PaymentOutcomeRecorder
}

func NewOrderServer(orders port.PaymentOutcomeRecorder) *OrderServer {
	return &OrderServer{orders: orders}
}
func (s *OrderServer) RecordPaymentOutcome(ctx context.Context, req *orderv1.RecordPaymentOutcomeRequest) (*orderv1.RecordPaymentOutcomeResponse, error) {
	state := ""
	switch req.GetOutcome() {
	case orderv1.PaymentOutcome_PAYMENT_OUTCOME_PAID:
		state = "PAID"
	case orderv1.PaymentOutcome_PAYMENT_OUTCOME_FAILED:
		state = "FAILED"
	}
	err := s.orders.RecordPaymentOutcome(ctx, port.PaymentOutcomeCommand{OrderID: req.GetOrderId(), PaymentID: req.GetPaymentId(),
		CustomerID: req.GetCustomerId(), AmountUSDCents: req.GetAmountUsdCents(), Currency: req.GetCurrency(), Status: state})
	if err != nil {
		var invalid *domain.ValidationError
		switch {
		case errors.As(err, &invalid):
			return nil, status.Error(codes.InvalidArgument, invalid.Error())
		case errors.Is(err, domain.ErrPaymentUnavailable):
			return nil, status.Error(codes.FailedPrecondition, "payment does not match an eligible order")
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "order deadline exceeded")
		default:
			slog.Error("record payment failed", "error", err, "service", "order-service")
			return nil, status.Error(codes.Unavailable, "order unavailable")
		}
	}
	return &orderv1.RecordPaymentOutcomeResponse{Acknowledged: true}, nil
}

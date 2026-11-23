package grpcadapter

import (
	"context"
	"errors"
	"log/slog"

	paymentv1 "ecommerce.local/contracts/payment/v1"
	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer
	payments port.PaymentCheckout
}

func NewPaymentServer(payments port.PaymentCheckout) *PaymentServer {
	return &PaymentServer{payments: payments}
}

func (s *PaymentServer) PreparePayment(ctx context.Context, req *paymentv1.PreparePaymentRequest) (*paymentv1.PaymentSession, error) {
	if req.GetReservationExpiresAt() == nil || req.GetReservationExpiresAt().CheckValid() != nil {
		return nil, status.Error(codes.InvalidArgument, "valid reservation expiry is required")
	}
	a, err := s.payments.PreparePayment(ctx, port.PreparePaymentCommand{
		OrderID: req.GetOrderId(), CustomerID: req.GetCustomerId(), ReservationID: req.GetReservationId(),
		ReservationExpiresAt: req.GetReservationExpiresAt().AsTime(), AmountUSDCents: req.GetAmountUsdCents(), Currency: req.GetCurrency(),
	})
	return paymentResponse(a, err)
}

func (s *PaymentServer) ClosePayment(ctx context.Context, req *paymentv1.PreparePaymentRequest) (*paymentv1.PaymentSession, error) {
	if req.GetReservationExpiresAt() == nil || req.GetReservationExpiresAt().CheckValid() != nil {
		return nil, status.Error(codes.InvalidArgument, "valid reservation expiry is required")
	}
	a, err := s.payments.ClosePayment(ctx, port.PreparePaymentCommand{
		OrderID: req.GetOrderId(), CustomerID: req.GetCustomerId(), ReservationID: req.GetReservationId(),
		ReservationExpiresAt: req.GetReservationExpiresAt().AsTime(), AmountUSDCents: req.GetAmountUsdCents(), Currency: req.GetCurrency(),
	})
	return paymentResponse(a, err)
}

func (s *PaymentServer) GetOrderPayment(ctx context.Context, req *paymentv1.GetOrderPaymentRequest) (*paymentv1.PaymentSession, error) {
	a, err := s.payments.GetOrderPayment(ctx, req.GetOrderId())
	return paymentResponse(a, err)
}

func paymentResponse(a domain.Payment, err error) (*paymentv1.PaymentSession, error) {
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			return nil, status.Error(codes.InvalidArgument, validationError.Error())
		case errors.Is(err, domain.ErrPaymentNotFound):
			return nil, status.Error(codes.NotFound, "payment not found")
		case errors.Is(err, domain.ErrReservationExpired), errors.Is(err, domain.ErrPaymentMismatch), errors.Is(err, domain.ErrPaymentUnavailable):
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		case errors.Is(err, context.Canceled):
			return nil, status.Error(codes.Canceled, "request canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.Error(codes.DeadlineExceeded, "payment deadline exceeded")
		default:
			slog.Error("prepare payment failed", "error", err, "service", "payment-service")
			return nil, status.Error(codes.Unavailable, "payment service unavailable")
		}
	}
	states := map[domain.PaymentStatus]paymentv1.PaymentStatus{
		domain.PaymentPending: paymentv1.PaymentStatus_PAYMENT_STATUS_PENDING,
		domain.PaymentPaid:    paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
		domain.PaymentFailed:  paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED,
	}
	result := &paymentv1.PaymentSession{PaymentId: a.ID, OrderId: a.OrderID, CustomerId: a.CustomerID,
		AmountUsdCents: a.AmountUSDCents, Currency: a.Currency, Status: states[a.Status], PaymentUrl: a.PaymentURL}
	if a.SessionExpiresAt != nil {
		result.SessionExpiresAt = timestamppb.New(*a.SessionExpiresAt)
	}
	return result, nil
}

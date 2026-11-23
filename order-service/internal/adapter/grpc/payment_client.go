package grpcadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	paymentv1 "ecommerce.local/contracts/payment/v1"
	"ecommerce.local/order-service/internal/port"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentClient struct {
	conn    *grpc.ClientConn
	client  paymentv1.PaymentServiceClient
	timeout time.Duration
}

var _ port.PaymentPreparer = (*PaymentClient)(nil)

func NewPaymentClient(address string, timeout time.Duration) (*PaymentClient, error) {
	if strings.TrimSpace(address) == "" || timeout <= 0 {
		return nil, fmt.Errorf("Payment address and positive timeout are required")
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("Payment grpc client: %w", err)
	}
	return &PaymentClient{conn: conn, client: paymentv1.NewPaymentServiceClient(conn), timeout: timeout}, nil
}

func (c *PaymentClient) Close() error { return c.conn.Close() }

func (c *PaymentClient) PreparePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentSession, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.client.PreparePayment(ctx, &paymentv1.PreparePaymentRequest{
		OrderId: req.OrderID, CustomerId: req.CustomerID, ReservationId: req.ReservationID,
		ReservationExpiresAt: timestamppb.New(req.ReservationExpiresAt), AmountUsdCents: req.AmountUSDCents, Currency: req.Currency,
	})
	if err != nil {
		return port.PaymentSession{}, paymentError(err)
	}
	return decodePayment(response)
}

func (c *PaymentClient) ClosePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentSession, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.client.ClosePayment(ctx, &paymentv1.PreparePaymentRequest{
		OrderId: req.OrderID, CustomerId: req.CustomerID, ReservationId: req.ReservationID,
		ReservationExpiresAt: timestamppb.New(req.ReservationExpiresAt), AmountUsdCents: req.AmountUSDCents, Currency: req.Currency,
	})
	if err != nil {
		return port.PaymentSession{}, paymentError(err)
	}
	return decodePayment(response)
}

func (c *PaymentClient) GetOrderPayment(ctx context.Context, orderID string) (port.PaymentSession, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.client.GetOrderPayment(ctx, &paymentv1.GetOrderPaymentRequest{OrderId: orderID})
	if err != nil {
		return port.PaymentSession{}, paymentError(err)
	}
	return decodePayment(response)
}

func paymentError(err error) error {
	switch status.Code(err) {
	case codes.NotFound:
		return port.ErrPaymentNotFound
	case codes.FailedPrecondition:
		return port.ErrPaymentUnavailable
	default:
		return &port.DependencyError{Service: "Payment", Err: err}
	}
}

func decodePayment(response *paymentv1.PaymentSession) (port.PaymentSession, error) {
	invalid := func() (port.PaymentSession, error) {
		return port.PaymentSession{}, &port.DependencyError{Service: "Payment", Err: fmt.Errorf("invalid payment response")}
	}
	if response == nil || response.GetPaymentId() == "" || response.GetOrderId() == "" || response.GetCustomerId() == "" {
		return invalid()
	}
	states := map[paymentv1.PaymentStatus]string{
		paymentv1.PaymentStatus_PAYMENT_STATUS_PENDING: "PENDING",
		paymentv1.PaymentStatus_PAYMENT_STATUS_PAID:    "PAID",
		paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED:  "FAILED",
	}
	state, ok := states[response.GetStatus()]
	if !ok {
		return invalid()
	}
	result := port.PaymentSession{PaymentID: response.GetPaymentId(), OrderID: response.GetOrderId(),
		CustomerID: response.GetCustomerId(), AmountUSDCents: response.GetAmountUsdCents(), Currency: response.GetCurrency(), Status: state, PaymentURL: response.GetPaymentUrl()}
	if response.GetSessionExpiresAt() != nil {
		if response.GetSessionExpiresAt().CheckValid() != nil {
			return invalid()
		}
		expiry := response.GetSessionExpiresAt().AsTime()
		result.SessionExpiresAt = &expiry
	}
	return result, nil
}

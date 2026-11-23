package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"ecommerce.local/contracts/events"
	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	amqp "github.com/rabbitmq/amqp091-go"
)

// PaymentConsumer is an inbound adapter. Order's saved state makes redelivery safe.
type PaymentConsumer struct {
	url    string
	orders port.PaymentOutcomeRecorder
}

func NewPaymentConsumer(url string, orders port.PaymentOutcomeRecorder) *PaymentConsumer {
	return &PaymentConsumer{url: url, orders: orders}
}

func (c *PaymentConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.consume(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("payment consumer interrupted; retrying", "error", err, "service", "order-service")
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *PaymentConsumer) consume(ctx context.Context) error {
	stop := func() bool { return false }
	defer func() { stop() }()
	conn, err := amqp.DialConfig(c.url, amqp.Config{Dial: func(network, address string) (net.Conn, error) {
		socket, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if err := socket.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			_ = socket.Close()
			return nil, err
		}
		// The library clears the handshake deadline after connecting. Cancellation
		// closes the socket to stop consumption and return unacknowledged messages.
		stop = context.AfterFunc(ctx, func() { _ = socket.Close() })
		return socket, nil
	}})
	if err != nil {
		return err
	}
	defer conn.Close()
	channel, err := conn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	if err := channel.ExchangeDeclare(events.PaymentExchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare(events.OrderPaymentQueue, true, false, false, false, nil); err != nil {
		return err
	}
	for _, key := range []string{events.PaymentPaid, events.PaymentFailed} {
		if err := channel.QueueBind(events.OrderPaymentQueue, key, events.PaymentExchange, false, nil); err != nil {
			return err
		}
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := channel.Consume(events.OrderPaymentQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("payment delivery channel closed")
			}
			workCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.record(workCtx, delivery)
			cancel()
			if err != nil {
				var invalid *domain.ValidationError
				if errors.As(err, &invalid) {
					slog.Error("invalid payment event rejected", "message_id", delivery.MessageId, "error", err, "service", "order-service")
					if err := delivery.Reject(false); err != nil {
						return err
					}
					continue
				}
				// End this attempt on failure; the reconnect delay avoids a hot retry loop.
				return errors.Join(err, delivery.Nack(false, true))
			}
			// The saved outcome schedules the existing worker for stock/cart work.
			if err := delivery.Ack(false); err != nil {
				return err
			}
		}
	}
}

func (c *PaymentConsumer) record(ctx context.Context, delivery amqp.Delivery) error {
	var event events.PaymentOutcome
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		return &domain.ValidationError{Field: "event", Message: "must be a payment outcome JSON object"}
	}
	if event.EventID != event.PaymentID+":"+event.Status || delivery.MessageId != event.EventID || delivery.RoutingKey != event.Type ||
		!((event.Type == events.PaymentPaid && event.Status == "PAID") || (event.Type == events.PaymentFailed && event.Status == "FAILED")) {
		return &domain.ValidationError{Field: "event", Message: "payment event identity, type and status must match"}
	}
	return c.orders.RecordPaymentOutcome(ctx, port.PaymentOutcomeCommand{
		OrderID: event.OrderID, PaymentID: event.PaymentID, CustomerID: event.CustomerID,
		AmountUSDCents: event.AmountUSDCents, Currency: event.Currency, Status: event.Status,
	})
}

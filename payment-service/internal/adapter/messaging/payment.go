package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"ecommerce.local/contracts/events"
	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	amqp "github.com/rabbitmq/amqp091-go"
)

type PaymentPublisher struct{ url string }

var _ port.PaymentPublisher = (*PaymentPublisher)(nil)

func NewPaymentPublisher(url string) *PaymentPublisher { return &PaymentPublisher{url: url} }

func (p *PaymentPublisher) PublishPaymentOutcome(ctx context.Context, outcome port.PaymentOutcome) error {
	kind := events.PaymentPaid
	if outcome.Status == domain.PaymentFailed {
		kind = events.PaymentFailed
	} else if outcome.Status != domain.PaymentPaid {
		return fmt.Errorf("only final payment outcomes may be published")
	}
	event := events.PaymentOutcome{
		EventID: outcome.PaymentID + ":" + string(outcome.Status), Type: kind,
		OrderID: outcome.OrderID, PaymentID: outcome.PaymentID, CustomerID: outcome.CustomerID,
		AmountUSDCents: outcome.AmountUSDCents, Currency: outcome.Currency, Status: string(outcome.Status),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Each attempt owns its connection; the existing worker retries failures.
	// Cancellation closes the socket, including during handshake or declaration.
	stop := func() bool { return false }
	defer func() { stop() }()
	conn, err := amqp.DialConfig(p.url, amqp.Config{Dial: func(network, address string) (net.Conn, error) {
		socket, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			stop = context.AfterFunc(ctx, func() { _ = socket.Close() })
		}
		return socket, err
	}})
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
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
	// Declare and bind before publishing so events are retained even before Order starts.
	if _, err := channel.QueueDeclare(events.OrderPaymentQueue, true, false, false, false, nil); err != nil {
		return err
	}
	for _, key := range []string{events.PaymentPaid, events.PaymentFailed} {
		if err := channel.QueueBind(events.OrderPaymentQueue, key, events.PaymentExchange, false, nil); err != nil {
			return err
		}
	}
	if err := channel.Confirm(false); err != nil {
		return err
	}
	returned := channel.NotifyReturn(make(chan amqp.Return, 1))
	confirmation, err := channel.PublishWithDeferredConfirmWithContext(ctx, events.PaymentExchange, kind, true, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: event.EventID, Type: kind, Body: body,
	})
	if err != nil {
		return err
	}
	confirmed, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("RabbitMQ did not confirm payment event")
	}
	select {
	case <-returned:
		return fmt.Errorf("payment event was not routed to Order's queue")
	default:
		return nil
	}
}

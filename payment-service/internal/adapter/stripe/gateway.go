package stripeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stripe/stripe-go/v84/webhook"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	stripe "github.com/stripe/stripe-go/v84"
)

type Gateway struct {
	client        *stripe.Client
	webhookSecret string
}

var _ port.StripeGateway = (*Gateway)(nil)

func NewGateway(secret, webhookSecret string, timeout time.Duration) (*Gateway, error) {
	if !strings.HasPrefix(secret, "sk_test_") || !strings.HasPrefix(webhookSecret, "whsec_") || timeout <= 0 {
		return nil, fmt.Errorf("Stripe test and webhook signing secrets and a positive timeout are required")
	}
	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		HTTPClient: &http.Client{Timeout: timeout}, MaxNetworkRetries: stripe.Int64(0),
	})
	return &Gateway{client: stripe.NewClient(secret, stripe.WithBackends(backends)), webhookSecret: webhookSecret}, nil
}

func (g *Gateway) CreateCheckoutSession(ctx context.Context, req port.CreateSessionRequest) (port.CheckoutSession, error) {
	metadata := map[string]string{"payment_id": req.PaymentID, "order_id": req.OrderID, "customer_id": req.CustomerID}
	params := &stripe.CheckoutSessionCreateParams{
		AdaptivePricing: &stripe.CheckoutSessionCreateAdaptivePricingParams{Enabled: stripe.Bool(false)},
		Mode:            stripe.String("payment"), ClientReferenceID: stripe.String(req.OrderID),
		SuccessURL: stripe.String(req.SuccessURL), CancelURL: stripe.String(req.CancelURL),
		ExpiresAt: stripe.Int64(req.ExpiresAt.Unix()), PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Metadata:          metadata,
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{Metadata: metadata},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1), PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency: stripe.String("usd"), UnitAmount: stripe.Int64(req.AmountUSDCents),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: stripe.String("Order " + req.OrderID)},
			},
		}},
	}
	params.SetIdempotencyKey(req.IdempotencyKey)
	params.AddExpand("payment_intent")
	session, err := g.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return port.CheckoutSession{}, fmt.Errorf("create Stripe session: %w", err)
	}
	return decodeSession(session)
}

func (g *Gateway) GetCheckoutSession(ctx context.Context, id string) (port.CheckoutSession, error) {
	params := &stripe.CheckoutSessionRetrieveParams{}
	params.AddExpand("payment_intent")
	session, err := g.client.V1CheckoutSessions.Retrieve(ctx, id, params)
	if err != nil {
		return port.CheckoutSession{}, fmt.Errorf("retrieve Stripe session: %w", err)
	}
	return decodeSession(session)
}

func decodeSession(s *stripe.CheckoutSession) (port.CheckoutSession, error) {
	if s == nil || s.ID == "" || s.Mode != stripe.CheckoutSessionModePayment || s.ExpiresAt <= 0 || s.ClientReferenceID != s.Metadata["order_id"] {
		return port.CheckoutSession{}, fmt.Errorf("invalid Stripe checkout session")
	}
	expiresAt := time.Unix(s.ExpiresAt, 0).UTC()
	result := port.CheckoutSession{
		SessionID: s.ID, PaymentID: s.Metadata["payment_id"], OrderID: s.Metadata["order_id"],
		CustomerID: s.Metadata["customer_id"], AmountUSDCents: s.AmountTotal, Currency: strings.ToUpper(string(s.Currency)),
		PaymentURL: s.URL, ExpiresAt: &expiresAt, LiveMode: s.Livemode, Status: domain.PaymentPending,
	}
	if s.PaymentIntent != nil {
		result.PaymentIntentID = s.PaymentIntent.ID
	}

	switch {
	case s.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid:
		result.Status = domain.PaymentPaid
	case s.PaymentIntent != nil && (s.PaymentIntent.Status == stripe.PaymentIntentStatusProcessing || s.PaymentIntent.Status == stripe.PaymentIntentStatusSucceeded):
		// An incomplete/uncertain settlement is never treated as failed or payable.
	case s.Status == stripe.CheckoutSessionStatusExpired && s.PaymentStatus == stripe.CheckoutSessionPaymentStatusUnpaid:
		result.Status = domain.PaymentFailed
	case s.Status == stripe.CheckoutSessionStatusOpen:
		result.Payable = true
	}
	if result.Payable {
		parsed, err := url.Parse(result.PaymentURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
			return port.CheckoutSession{}, fmt.Errorf("invalid Stripe payment URL")
		}
	} else {
		result.PaymentURL = ""
	}
	return result, nil
}

func (g *Gateway) VerifyWebhook(payload []byte, signature string) (port.VerifiedWebhook, error) {
	// Only signed object identity and our metadata marker are read here. Payment fields are retrieved
	// through our pinned API version, so event-version deserialization is unnecessary.
	event, err := webhook.ConstructEventWithOptions(payload, signature, g.webhookSecret, webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil || event.ID == "" || event.Livemode {
		return port.VerifiedWebhook{}, domain.ErrInvalidWebhook
	}
	verified := port.VerifiedWebhook{EventID: event.ID, EventType: string(event.Type), LiveMode: event.Livemode}
	switch string(event.Type) {
	case "checkout.session.completed", "checkout.session.expired", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed":
		verified.Supported = true
	default:
		return verified, nil
	}
	if event.Data == nil {
		return verified, domain.ErrInvalidWebhook
	}
	var object struct {
		ID       string            `json:"id"`
		Object   string            `json:"object"`
		Metadata map[string]string `json:"metadata"`
	}
	if json.Unmarshal(event.Data.Raw, &object) != nil || object.Object != "checkout.session" || object.ID == "" {
		return verified, domain.ErrInvalidWebhook
	}
	if object.Metadata["payment_id"] == "" {
		verified.Supported = false
	}
	verified.SessionID = object.ID
	return verified, nil
}

func (g *Gateway) ExpireCheckoutSession(ctx context.Context, id string) (port.CheckoutSession, error) {
	session, err := g.client.V1CheckoutSessions.Expire(ctx, id, &stripe.CheckoutSessionExpireParams{})
	if err != nil {
		return port.CheckoutSession{}, fmt.Errorf("expire Stripe session: %w", err)
	}
	return decodeSession(session)
}

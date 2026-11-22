package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
)

type CheckoutHandler struct {
	checkout port.Checkout
	timeout  time.Duration
}

func NewCheckoutHandler(checkout port.Checkout, timeout time.Duration) *CheckoutHandler {
	return &CheckoutHandler{checkout: checkout, timeout: timeout}
}

type orderItemResponse struct {
	ProductID    string `json:"product_id"`
	ProductName  string `json:"product_name"`
	Quantity     int32  `json:"quantity"`
	UnitPriceUSD string `json:"unit_price_usd"`
}

type checkoutResponse struct {
	OrderID              string              `json:"order_id"`
	Status               string              `json:"status"`
	Currency             string              `json:"currency"`
	TotalUSD             string              `json:"total_usd"`
	ReservationID        string              `json:"reservation_id"`
	ReservationExpiresAt *time.Time          `json:"reservation_expires_at"`
	Items                []orderItemResponse `json:"items"`
}

type apiError struct {
	Code    string `json:"error_code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type envelope struct {
	Data   any        `json:"data"`
	Errors []apiError `json:"errors"`
}

func (h *CheckoutHandler) Start(w http.ResponseWriter, r *http.Request) {
	// Checkout always uses the saved cart and verified gateway identity. The optional
	// body is an empty object: clients cannot submit prices, quantities or an owner.
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request *struct{}
	if err := decoder.Decode(&request); err != io.EOF {
		if err != nil || request == nil {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "expected an empty JSON object or no request body", "")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
			return
		}
	}

	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "exactly one Idempotency-Key header is required", "idempotency_key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	order, err := h.checkout.Start(ctx, port.CheckoutCommand{CustomerID: r.Header.Get("X-User-ID"), IdempotencyKey: keys[0]})

	if order.Status == domain.OrderPending || order.Status == domain.OrderCleanupPending {
		if err != nil {
			slog.Warn("checkout queued for recovery", "order_id", order.ID, "error", err, "correlation_id", r.Header.Get("X-Correlation-Id"), "service", "order-service")
		}
		w.Header().Set("Retry-After", "5")
		respondJSON(w, http.StatusAccepted, envelope{Data: checkoutProgressResponse{OrderID: order.ID, Status: order.Status}, Errors: []apiError{}})
		return
	}
	if err != nil {
		var validationError *domain.ValidationError
		var dependencyError *port.DependencyError
		switch {
		case errors.As(err, &dependencyError), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			respondError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "checkout is temporarily unavailable; retry with the same idempotency key", "")
		case errors.As(err, &validationError):
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		case errors.Is(err, domain.ErrEmptyCart):
			respondError(w, http.StatusConflict, "EMPTY_CART", err.Error(), "")
		case errors.Is(err, domain.ErrProductUnavailable):
			respondError(w, http.StatusConflict, "PRODUCT_UNAVAILABLE", err.Error(), "")
		default:
			slog.Error("checkout failed", "error", err, "correlation_id", r.Header.Get("X-Correlation-Id"), "service", "order-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	if order.Status == domain.OrderCheckoutFailed {
		message := "checkout could not be completed"
		switch order.FailureCode {
		case "EMPTY_CART":
			message = domain.ErrEmptyCart.Error()
		case "PRODUCT_UNAVAILABLE":
			message = domain.ErrProductUnavailable.Error()
		case "INSUFFICIENT_STOCK":
			message = domain.ErrInsufficientStock.Error()
		case "RESERVATION_UNAVAILABLE":
			message = domain.ErrReservationUnavailable.Error()
		}
		respondError(w, http.StatusConflict, order.FailureCode, message, "")
		return
	}
	if order.Status != domain.OrderAwaitingPayment {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "checkout returned an invalid outcome", "")
		return
	}
	items := make([]orderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemResponse{ProductID: item.ProductID, ProductName: item.ProductName, Quantity: item.Quantity, UnitPriceUSD: item.UnitPriceUSD})
	}
	respondJSON(w, http.StatusCreated, envelope{
		Data: checkoutResponse{OrderID: order.ID, Status: order.Status, Currency: order.Currency, TotalUSD: order.TotalUSD,
			ReservationID: order.ReservationID, ReservationExpiresAt: order.ReservationExpiresAt, Items: items},
		Errors: []apiError{},
	})
}

func respondError(w http.ResponseWriter, status int, code, message, field string) {
	respondJSON(w, status, envelope{Data: nil, Errors: []apiError{{Code: code, Message: message, Field: field}}})
}

func respondJSON(w http.ResponseWriter, status int, value envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type checkoutProgressResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

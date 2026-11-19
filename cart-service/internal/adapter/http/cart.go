package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"ecommerce.local/cart-service/internal/domain"
	"ecommerce.local/cart-service/internal/port"
)

type CartHandler struct{ cart port.Cart }

func NewCartHandler(cart port.Cart) *CartHandler { return &CartHandler{cart: cart} }

type addCartItemRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

type cartItemResponse struct {
	CartID    string `json:"cart_id"`
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
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

func (h *CartHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	var request *addCartItemRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request == nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	item, err := h.cart.AddItem(r.Context(), port.AddCartItemCommand{
		CustomerID: r.Header.Get("X-User-ID"), ProductID: request.ProductID, Quantity: request.Quantity,
	})
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		case errors.Is(err, domain.ErrQuantityLimit):
			respondError(w, http.StatusConflict, "QUANTITY_LIMIT_EXCEEDED", err.Error(), "quantity")
		default:
			slog.Error("add cart item failed", "error", err, "service", "cart-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	respondJSON(w, http.StatusOK, envelope{
		Data:   cartItemResponse{CartID: item.CartID, ProductID: item.ProductID, Quantity: item.Quantity},
		Errors: []apiError{},
	})
}

func respondError(w http.ResponseWriter, status int, code, message, field string) {
	respondJSON(w, status, envelope{Data: nil, Errors: []apiError{{Code: code, Message: message, Field: field}}})
}

func respondJSON(w http.ResponseWriter, status int, value envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type savedCartItemResponse struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type savedCartResponse struct {
	Items []savedCartItemResponse `json:"items"`
}

func (h *CartHandler) View(w http.ResponseWriter, r *http.Request) {
	items, err := h.cart.View(r.Context(), r.Header.Get("X-User-ID"))
	if err != nil {
		var validationError *domain.ValidationError
		if errors.As(err, &validationError) {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		} else {
			slog.Error("view saved cart failed", "error", err, "service", "cart-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	result := make([]savedCartItemResponse, 0, len(items))
	for _, item := range items {
		result = append(result, savedCartItemResponse{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	respondJSON(w, http.StatusOK, envelope{Data: savedCartResponse{Items: result}, Errors: []apiError{}})
}

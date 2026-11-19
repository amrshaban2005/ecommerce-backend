package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"ecommerce.local/product-service/internal/domain"
	"ecommerce.local/product-service/internal/port"
)

type ProductHandler struct{ products port.ProductCatalog }

func NewProductHandler(products port.ProductCatalog) *ProductHandler {
	return &ProductHandler{products: products}
}

type productResponse struct {
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	PriceUSD     string `json:"price_usd"`
	ImageURL     string `json:"image_url,omitempty"`
}

type createProductRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CategoryID  string `json:"category_id"`
	PriceUSD    string `json:"price_usd"`
	ImageURL    string `json:"image_url"`
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

func (h *ProductHandler) Browse(w http.ResponseWriter, r *http.Request) {
	products, err := h.products.Browse(r.Context())
	if err != nil {
		slog.Error("browse products failed", "error", err, "service", "product-service")
		respondJSON(w, http.StatusInternalServerError, envelope{Data: nil, Errors: []apiError{{Code: "INTERNAL_ERROR", Message: "internal server error"}}})
		return
	}
	result := make([]productResponse, 0, len(products))
	for _, product := range products {
		result = append(result, productResponse{ProductID: product.ID, Name: product.Name, Description: product.Description, CategoryID: product.CategoryID, CategoryName: product.CategoryName, PriceUSD: product.PriceUSD, ImageURL: product.ImageURL})
	}
	respondJSON(w, http.StatusOK, envelope{Data: result, Errors: []apiError{}})
}

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	var request *createProductRequest
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
	product, err := h.products.Create(r.Context(), port.CreateProductCommand{
		Name: request.Name, Description: request.Description, CategoryID: request.CategoryID,
		PriceUSD: request.PriceUSD, ImageURL: request.ImageURL,
	})
	if err != nil {
		var validationError *domain.ValidationError
		if errors.As(err, &validationError) {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		} else {
			slog.Error("create product failed", "error", err, "service", "product-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	respondJSON(w, http.StatusCreated, envelope{
		Data: productResponse{
			ProductID: product.ID, Name: product.Name, Description: product.Description,
			CategoryID: product.CategoryID, CategoryName: product.CategoryName,
			PriceUSD: product.PriceUSD, ImageURL: product.ImageURL,
		},
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

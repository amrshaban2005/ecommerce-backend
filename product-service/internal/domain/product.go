package domain

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Product contains catalog data owned by Product service. Inventory is owned elsewhere.
// PriceUSD preserves PostgreSQL NUMERIC precision without floating-point conversion.
type Product struct {
	ID           string
	Name         string
	Description  string
	CategoryID   string
	CategoryName string
	PriceUSD     string
	Active       bool
	ImageURL     string
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

var decimalPrice = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// NormalizeProduct validates catalog input without converting the USD price to floating point.
func NormalizeProduct(name, description, categoryID, priceUSD, imageURL string) (Product, error) {
	product := Product{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		CategoryID:  strings.TrimSpace(categoryID),
		PriceUSD:    strings.TrimSpace(priceUSD),
		ImageURL:    strings.TrimSpace(imageURL),
	}
	if product.Name == "" {
		return Product{}, &ValidationError{Field: "name", Message: "is required"}
	}
	if product.Description == "" {
		return Product{}, &ValidationError{Field: "description", Message: "is required"}
	}
	category, err := uuid.Parse(product.CategoryID)
	if err != nil {
		return Product{}, &ValidationError{Field: "category_id", Message: "must be a valid UUID"}
	}
	product.CategoryID = category.String()
	if !decimalPrice.MatchString(product.PriceUSD) || !strings.ContainsAny(product.PriceUSD, "123456789") {
		return Product{}, &ValidationError{Field: "price_usd", Message: "must be a positive decimal value"}
	}
	return product, nil
}

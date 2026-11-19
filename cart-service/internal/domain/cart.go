package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

const MaxQuantity = 2147483647

var ErrQuantityLimit = errors.New("resulting quantity exceeds the supported limit")

type CartItem struct {
	CartID    string
	ProductID string
	Quantity  int32
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func NormalizeCartItem(customerID, productID string, quantity int64) (string, string, error) {
	customerID, err := NormalizeCustomerID(customerID)
	if err != nil {
		return "", "", err
	}
	product, err := uuid.Parse(strings.TrimSpace(productID))
	if err != nil || product == uuid.Nil {
		return "", "", &ValidationError{Field: "product_id", Message: "must be a valid non-zero UUID"}
	}
	if quantity <= 0 || quantity > MaxQuantity {
		return "", "", &ValidationError{Field: "quantity", Message: "must be a positive integer no greater than 2147483647"}
	}
	return customerID, product.String(), nil
}

func NormalizeCustomerID(customerID string) (string, error) {
	customer, err := uuid.Parse(strings.TrimSpace(customerID))
	if err != nil || customer == uuid.Nil {
		return "", &ValidationError{Field: "customer_id", Message: "must be a valid non-zero UUID"}
	}
	return customer.String(), nil
}

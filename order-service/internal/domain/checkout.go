package domain

import (
	"errors"
	"math/big"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	OrderPending         = "PENDING"
	OrderAwaitingPayment = "AWAITING_PAYMENT"
	OrderCheckoutFailed  = "CHECKOUT_FAILED"
	OrderCleanupPending  = "CLEANUP_PENDING"
)

var (
	ErrEmptyCart              = errors.New("cart is empty")
	ErrProductUnavailable     = errors.New("a cart product is missing or inactive")
	ErrInsufficientStock      = errors.New("insufficient stock for checkout")
	ErrReservationUnavailable = errors.New("checkout reservation is unavailable")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

var decimalPrice = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func ValidateIdempotencyKey(value string) error {
	if !idempotencyKeyPattern.MatchString(value) {
		return &ValidationError{Field: "idempotency_key", Message: "must contain 1 to 128 letters, digits, dots, underscores, colons or hyphens"}
	}
	return nil
}

func NormalizeCustomerID(value string) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return "", &ValidationError{Field: "customer_id", Message: "must be a valid non-zero UUID"}
	}
	return id.String(), nil
}

// NewPendingOrder validates the snapshot and calculates its exact USD total.
// Prices retain catalog precision; checkout never converts money to floating point.
func NewPendingOrder(id, customerID string, items []OrderItem) (Order, error) {
	customerID, err := NormalizeCustomerID(customerID)
	if err != nil {
		return Order{}, err
	}
	if len(items) == 0 {
		return Order{}, ErrEmptyCart
	}
	if len(items) > 1000 {
		return Order{}, &ValidationError{Field: "items", Message: "must contain at most 1000 items"}
	}
	total := new(big.Rat)
	scale := 2
	snapshot := make([]OrderItem, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		productID, err := uuid.Parse(strings.TrimSpace(item.ProductID))
		if err != nil || productID == uuid.Nil {
			return Order{}, &ValidationError{Field: "product_id", Message: "must be a valid non-zero UUID"}
		}
		item.ProductID = productID.String()
		if seen[item.ProductID] {
			return Order{}, &ValidationError{Field: "items", Message: "must not contain duplicate products"}
		}
		seen[item.ProductID] = true
		if item.Quantity <= 0 {
			return Order{}, &ValidationError{Field: "quantity", Message: "must be a positive integer"}
		}
		item.UnitPriceUSD = strings.TrimSpace(item.UnitPriceUSD)
		if !decimalPrice.MatchString(item.UnitPriceUSD) {
			return Order{}, &ValidationError{Field: "unit_price_usd", Message: "must be a positive decimal"}
		}
		price, ok := new(big.Rat).SetString(item.UnitPriceUSD)
		if !ok || price.Sign() <= 0 {
			return Order{}, &ValidationError{Field: "unit_price_usd", Message: "must be a positive decimal"}
		}
		if dot := strings.IndexByte(item.UnitPriceUSD, '.'); dot >= 0 {
			scale = max(scale, len(item.UnitPriceUSD)-dot-1)
		}
		line := new(big.Rat).Mul(price, new(big.Rat).SetInt64(int64(item.Quantity)))
		total.Add(total, line)
		snapshot = append(snapshot, item)
	}
	return Order{ID: id, CustomerID: customerID, Status: OrderPending, Currency: "USD", TotalUSD: total.FloatString(scale), Items: snapshot}, nil
}

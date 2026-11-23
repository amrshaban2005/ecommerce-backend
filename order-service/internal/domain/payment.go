package domain

import (
	"errors"
	"math/big"
	"strings"
)

var ErrPaymentUnavailable = errors.New("payment cannot be prepared for this order")

// AmountUSDCents converts the saved total exactly; fractional cents are rejected.
// Current card checkout supports Stripe's USD amount range, without rounding.
func AmountUSDCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	invalid := &ValidationError{Field: "total_usd", Message: "must be a whole-cent USD total between 0.50 and 999999.99"}
	if !decimalPrice.MatchString(value) {
		return 0, invalid
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, invalid
	}
	amount.Mul(amount, big.NewRat(100, 1))
	if !amount.IsInt() || !amount.Num().IsInt64() {
		return 0, invalid
	}
	cents := amount.Num().Int64()
	if cents < 50 || cents > 99999999 {
		return 0, invalid
	}
	return cents, nil
}

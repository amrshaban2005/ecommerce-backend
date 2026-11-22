package port

import "errors"

var (
	ErrInsufficientStock      = errors.New("inventory has insufficient stock")
	ErrReservationNotFound    = errors.New("inventory reservation not found")
	ErrReservationConflict    = errors.New("inventory reservation conflicts with this request")
	ErrReservationUnavailable = errors.New("inventory reservation is released or expired")
)

// DependencyError keeps transport errors inside the adapter while allowing callers
// to distinguish an unavailable/invalid dependency from a business rejection.
type DependencyError struct {
	Service string
	Err     error
}

func (e *DependencyError) Error() string { return e.Service + " dependency: " + e.Err.Error() }
func (e *DependencyError) Unwrap() error { return e.Err }

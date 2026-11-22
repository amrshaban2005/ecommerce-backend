package domain

import "time"

// Monetary values are exact USD decimal strings backed by PostgreSQL NUMERIC.
// Order snapshots are independent of later catalog edits.
type OrderItem struct {
	ProductID    string
	ProductName  string
	Quantity     int32
	UnitPriceUSD string
}

type Order struct {
	IdempotencyKey       string
	FailureCode          string
	ID                   string
	CustomerID           string
	Currency             string
	Status               string
	TotalUSD             string
	ReservationID        string
	ReservationExpiresAt *time.Time
	Items                []OrderItem
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"ecommerce.local/inventory-service/internal/domain"
	"ecommerce.local/inventory-service/internal/port"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresReservationRepository struct{ pool *pgxpool.Pool }

func NewPostgresReservationRepository(pool *pgxpool.Pool) port.ReservationRepository {
	return &PostgresReservationRepository{pool: pool}
}

type reservationReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readReservation(ctx context.Context, q reservationReader, orderID string, lock bool) (domain.Reservation, error) {
	sql := `SELECT id, order_id, status, expires_at FROM reservations WHERE order_id = $1`
	if lock {
		sql += ` FOR UPDATE`
	}
	var reservation domain.Reservation
	err := q.QueryRow(ctx, sql, orderID).Scan(&reservation.ID, &reservation.OrderID, &reservation.Status, &reservation.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("read reservation: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT product_id, quantity FROM reservation_items WHERE reservation_id = $1 ORDER BY product_id`, reservation.ID)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("read reservation items: %w", err)
	}
	defer rows.Close()
	reservation.Items = make([]domain.ReservationItem, 0)
	for rows.Next() {
		var item domain.ReservationItem
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			return domain.Reservation{}, fmt.Errorf("scan reservation item: %w", err)
		}
		reservation.Items = append(reservation.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Reservation{}, fmt.Errorf("iterate reservation items: %w", err)
	}
	return reservation, nil
}

func (r *PostgresReservationRepository) Reserve(ctx context.Context, reservationID, orderID string, items []domain.ReservationItem) (domain.Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("begin reservation: %w", err)
	}
	defer tx.Rollback(ctx)

	inserted, err := tx.Exec(ctx, `
 INSERT INTO reservations (id, order_id, status, expires_at)
 VALUES ($1, $2, 'ACTIVE', clock_timestamp() + INTERVAL '15 minutes')
 ON CONFLICT (order_id) DO NOTHING`, reservationID, orderID)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("create reservation: %w", err)
	}
	reservation, err := readReservation(ctx, tx, orderID, true)
	if err != nil {
		return domain.Reservation{}, err
	}
	if inserted.RowsAffected() == 0 {
		if reservation.Status == domain.ReservationReleased {
			return domain.Reservation{}, domain.ErrReservationUnavailable
		}
		if !slices.Equal(reservation.Items, items) {
			return domain.Reservation{}, domain.ErrReservationConflict
		}
		// Expiry alone does not release held stock. Payment-aware cleanup comes later.
		var expired bool
		if err := tx.QueryRow(ctx, `SELECT expires_at <= clock_timestamp() FROM reservations WHERE id = $1`, reservation.ID).Scan(&expired); err != nil {
			return domain.Reservation{}, fmt.Errorf("check existing reservation expiry: %w", err)
		}
		if reservation.Status != domain.ReservationActive || expired {
			return domain.Reservation{}, domain.ErrReservationUnavailable
		}
		return reservation, nil
	}

	for _, item := range items {
		updated, err := tx.Exec(ctx, `
  UPDATE inventory SET reserved = reserved + $2
  WHERE product_id = $1 AND on_hand - reserved >= $2`, item.ProductID, item.Quantity)
		if err != nil {
			return domain.Reservation{}, fmt.Errorf("reserve product stock: %w", err)
		}
		if updated.RowsAffected() == 0 {
			return domain.Reservation{}, domain.ErrInsufficientStock
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reservation_items (reservation_id, product_id, quantity) VALUES ($1, $2, $3)`, reservation.ID, item.ProductID, item.Quantity); err != nil {
			return domain.Reservation{}, fmt.Errorf("save reservation item: %w", err)
		}
	}
	reservation.Items = items
	// Fail rather than returning an already-expired hold after prolonged contention.
	var expired bool
	if err := tx.QueryRow(ctx, `SELECT expires_at <= clock_timestamp() FROM reservations WHERE id = $1`, reservation.ID).Scan(&expired); err != nil {
		return domain.Reservation{}, fmt.Errorf("check reservation expiry: %w", err)
	}
	if expired {
		return domain.Reservation{}, domain.ErrReservationUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, fmt.Errorf("commit reservation: %w", err)
	}
	return reservation, nil
}

func (r *PostgresReservationRepository) GetByOrder(ctx context.Context, orderID string) (domain.Reservation, error) {
	return readReservation(ctx, r.pool, orderID, false)
}

func (r *PostgresReservationRepository) ReleaseByOrder(ctx context.Context, reservationID, orderID string) (domain.Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, fmt.Errorf("begin reservation release: %w", err)
	}

	defer tx.Rollback(ctx)
	// Cancellation is persisted before looking up stock. The unique order key
	// serializes this insert against an in-flight or later ReserveStock call.
	if _, err := tx.Exec(ctx, `
 INSERT INTO reservations (id, order_id, status, expires_at, released_at)
 VALUES ($1, $2, 'RELEASED', clock_timestamp(), clock_timestamp())
 ON CONFLICT (order_id) DO NOTHING`, reservationID, orderID); err != nil {
		return domain.Reservation{}, fmt.Errorf("record reservation cancellation: %w", err)
	}
	reservation, err := readReservation(ctx, tx, orderID, true)
	if err != nil {
		return domain.Reservation{}, err
	}
	if reservation.Status == domain.ReservationFinalized {
		return domain.Reservation{}, domain.ErrReservationUnavailable
	}
	if reservation.Status == domain.ReservationReleased {
		if err := tx.Commit(ctx); err != nil {
			return domain.Reservation{}, fmt.Errorf("commit reservation cancellation: %w", err)
		}
		return reservation, nil
	}
	for _, item := range reservation.Items {
		updated, err := tx.Exec(ctx, `UPDATE inventory SET reserved = reserved - $2 WHERE product_id = $1 AND reserved >= $2`, item.ProductID, item.Quantity)
		if err != nil {
			return domain.Reservation{}, fmt.Errorf("release product stock: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return domain.Reservation{}, fmt.Errorf("reserved stock inconsistent for product %s", item.ProductID)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status = 'RELEASED', released_at = clock_timestamp() WHERE id = $1`, reservation.ID); err != nil {
		return domain.Reservation{}, fmt.Errorf("mark reservation released: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, fmt.Errorf("commit reservation release: %w", err)
	}
	reservation.Status = domain.ReservationReleased
	return reservation, nil
}

func (r *PostgresReservationRepository) FinalizeByOrder(ctx context.Context, orderID string) (domain.Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, err
	}
	defer tx.Rollback(ctx)
	reservation, err := readReservation(ctx, tx, orderID, true)
	if err != nil {
		return reservation, err
	}
	if reservation.Status == domain.ReservationFinalized {
		return reservation, nil
	}
	if reservation.Status != domain.ReservationActive {
		return reservation, domain.ErrReservationUnavailable
	}
	// A verified paid order may finish after the hold deadline. Expiry alone never
	// releases stock, so an active reservation can still be finalized safely.
	for _, item := range reservation.Items {
		updated, err := tx.Exec(ctx, `UPDATE inventory SET on_hand=on_hand-$2,reserved=reserved-$2
   WHERE product_id=$1 AND on_hand>=$2 AND reserved>=$2`, item.ProductID, item.Quantity)
		if err != nil {
			return reservation, err
		}
		if updated.RowsAffected() != 1 {
			return reservation, fmt.Errorf("reserved stock inconsistent for %s", item.ProductID)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status='FINALIZED' WHERE id=$1`, reservation.ID); err != nil {
		return reservation, err
	}
	if err := tx.Commit(ctx); err != nil {
		return reservation, err
	}
	reservation.Status = domain.ReservationFinalized
	return reservation, nil
}

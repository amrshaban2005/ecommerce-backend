package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ecommerce.local/order-service/internal/domain"
	"ecommerce.local/order-service/internal/port"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresOrderRepository struct{ pool *pgxpool.Pool }

func NewPostgresOrderRepository(pool *pgxpool.Pool) port.OrderRepository {
	return &PostgresOrderRepository{pool: pool}
}

func (r *PostgresOrderRepository) FindByKey(ctx context.Context, customerID, key string) (domain.Order, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM orders WHERE customer_id = $1 AND idempotency_key = $2`, customerID, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, port.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("find checkout order: %w", err)
	}
	return r.LoadOrder(ctx, id)
}

func (r *PostgresOrderRepository) SavePending(ctx context.Context, order domain.Order, retryDelay time.Duration) (domain.Order, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("begin order creation: %w", err)
	}
	defer tx.Rollback(ctx)
	inserted, err := tx.Exec(ctx, `
 INSERT INTO orders (id, customer_id, status, currency, total_usd, idempotency_key, retry_after)
 VALUES ($1, $2, 'PENDING', $3, $4::numeric, $5, clock_timestamp() + $6::bigint * INTERVAL '1 second')
 ON CONFLICT (customer_id, idempotency_key) DO NOTHING`, order.ID, order.CustomerID, order.Currency, order.TotalUSD,
		order.IdempotencyKey, int64(retryDelay/time.Second))
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("save pending order: %w", err)
	}
	if inserted.RowsAffected() == 1 {
		for _, item := range order.Items {
			if _, err := tx.Exec(ctx, `INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price_usd)
   VALUES ($1, $2, $3, $4, $5::numeric)`, order.ID, item.ProductID, item.ProductName, item.Quantity, item.UnitPriceUSD); err != nil {
				return domain.Order{}, false, fmt.Errorf("save order price snapshot: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, false, fmt.Errorf("commit order creation: %w", err)
	}
	// A concurrent request may have saved the same key with a different generated ID.
	saved, err := r.FindByKey(ctx, order.CustomerID, order.IdempotencyKey)
	return saved, inserted.RowsAffected() == 1, err
}

func (r *PostgresOrderRepository) LoadOrder(ctx context.Context, orderID string) (domain.Order, error) {
	var order domain.Order
	err := r.pool.QueryRow(ctx, `SELECT id, customer_id, status, currency, total_usd::text,
 COALESCE(reservation_id::text, ''), reservation_expires_at, COALESCE(idempotency_key, ''), COALESCE(failure_code, '')
 FROM orders WHERE id = $1`, orderID).
		Scan(&order.ID, &order.CustomerID, &order.Status, &order.Currency, &order.TotalUSD, &order.ReservationID,
			&order.ReservationExpiresAt, &order.IdempotencyKey, &order.FailureCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, port.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("load checkout order: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT product_id, product_name, quantity, unit_price_usd::text
 FROM order_items WHERE order_id = $1 ORDER BY product_id`, orderID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("load checkout snapshot: %w", err)
	}
	defer rows.Close()
	order.Items = make([]domain.OrderItem, 0)
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.Quantity, &item.UnitPriceUSD); err != nil {
			return domain.Order{}, fmt.Errorf("read checkout snapshot: %w", err)
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Order{}, fmt.Errorf("iterate checkout snapshot: %w", err)
	}
	return order, nil
}

func (r *PostgresOrderRepository) CompleteCheckout(ctx context.Context, orderID, reservationID string, expiresAt time.Time) (bool, error) {
	updated, err := r.pool.Exec(ctx, `UPDATE orders SET status = 'AWAITING_PAYMENT', reservation_id = $2,
 reservation_expires_at = $3, retry_after = NULL, last_error = NULL, updated_at = clock_timestamp()
 WHERE id = $1 AND status = 'PENDING' AND $3::timestamptz > clock_timestamp()`, orderID, reservationID, expiresAt)
	if err != nil {
		return false, fmt.Errorf("complete checkout: %w", err)
	}
	return updated.RowsAffected() == 1, nil
}

func (r *PostgresOrderRepository) MarkCleanup(ctx context.Context, orderID, failureCode string) (bool, error) {
	// This competes atomically with completion. Cleanup can never claim an order
	// that has already become ready for payment.
	updated, err := r.pool.Exec(ctx, `UPDATE orders SET status = 'CLEANUP_PENDING', failure_code = $2,
 retry_after = clock_timestamp(), updated_at = clock_timestamp() WHERE id = $1 AND status = 'PENDING'`, orderID, failureCode)
	if err != nil {
		return false, fmt.Errorf("mark order cleanup: %w", err)
	}
	return updated.RowsAffected() == 1, nil
}

func (r *PostgresOrderRepository) FailCheckout(ctx context.Context, orderID string) (bool, error) {
	updated, err := r.pool.Exec(ctx, `UPDATE orders SET status = 'CHECKOUT_FAILED', retry_after = NULL,
 last_error = NULL, updated_at = clock_timestamp() WHERE id = $1 AND status = 'CLEANUP_PENDING'`, orderID)
	if err != nil {
		return false, fmt.Errorf("mark checkout failed: %w", err)
	}
	return updated.RowsAffected() == 1, nil
}

package repository

import (
	"context"
	"fmt"
	"time"
)

func (r *PostgresOrderRepository) ListRecovery(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM orders
 WHERE status IN ('PENDING', 'CLEANUP_PENDING') AND COALESCE(retry_after, created_at) <= clock_timestamp()
 ORDER BY COALESCE(retry_after, created_at), id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list unfinished orders: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read unfinished order: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unfinished orders: %w", err)
	}
	return ids, nil
}

func (r *PostgresOrderRepository) RetryCheckout(ctx context.Context, orderID, message string, delay time.Duration) error {
	text := []rune(message)
	if len(text) > 1024 {
		message = string(text[:1024])
	}
	_, err := r.pool.Exec(ctx, `UPDATE orders SET retry_after = clock_timestamp() + $2::bigint * INTERVAL '1 second',
 last_error = $3, updated_at = clock_timestamp() WHERE id = $1 AND status IN ('PENDING', 'CLEANUP_PENDING')`,
		orderID, int64(delay/time.Second), message)
	if err != nil {
		return fmt.Errorf("schedule order retry: %w", err)
	}
	return nil
}

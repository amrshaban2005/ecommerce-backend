package repository

import (
	"context"
	"fmt"

	"ecommerce.local/order-service/internal/port"
)

func (r *PostgresOrderRepository) RecordPaymentOutcome(ctx context.Context, p port.PaymentOutcomeCommand) (bool, error) {
	// Same-payment notifications may repeat or arrive out of order. PAID wins over
	// an older failure, and COMPLETED is never moved backwards. No stock changes here.
	updated, err := r.pool.Exec(ctx, `UPDATE orders SET payment_id=$2,
 payment_status=CASE WHEN payment_status='PAID' OR status IN ('PAID','COMPLETED') THEN 'PAID' ELSE $6 END,
 status=CASE WHEN status='COMPLETED' THEN status WHEN $6='PAID' THEN 'PAID' ELSE status END,
 retry_after=CASE WHEN status='COMPLETED' THEN NULL ELSE clock_timestamp() END,updated_at=clock_timestamp()
 WHERE id=$1 AND customer_id=$3 AND total_usd*100=$4::bigint AND currency=$5
 AND status IN ('AWAITING_PAYMENT','PAID','COMPLETED','CLEANUP_PENDING','EXPIRED','CHECKOUT_FAILED') AND (payment_id IS NULL OR payment_id=$2)`,
		p.OrderID, p.PaymentID, p.CustomerID, p.AmountUSDCents, p.Currency, p.Status)
	if err != nil {
		return false, fmt.Errorf("record payment outcome: %w", err)
	}
	return updated.RowsAffected() == 1, nil
}

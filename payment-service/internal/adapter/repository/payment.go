package repository

import (
	"context"
	"errors"
	"fmt"

	"ecommerce.local/payment-service/internal/domain"
	"ecommerce.local/payment-service/internal/port"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresPaymentRepository struct{ pool *pgxpool.Pool }

func NewPostgresPaymentRepository(pool *pgxpool.Pool) port.PaymentRepository {
	return &PostgresPaymentRepository{pool: pool}
}

const paymentColumns = `id, order_id, customer_id, reservation_id, reservation_expires_at,
 amount_usd_cents, currency, status, provider_idempotency_key, COALESCE(provider_session_id, ''),
 COALESCE(provider_payment_intent_id, ''), COALESCE(payment_url, ''), session_expires_at,
 success_url, cancel_url, created_at, event_published`

type rowScanner interface{ Scan(...any) error }

func scanPayment(row rowScanner) (domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(&p.ID, &p.OrderID, &p.CustomerID, &p.ReservationID, &p.ReservationExpiresAt,
		&p.AmountUSDCents, &p.Currency, &p.Status, &p.ProviderIdempotencyKey, &p.ProviderSessionID,
		&p.ProviderPaymentIntentID, &p.PaymentURL, &p.SessionExpiresAt, &p.SuccessURL, &p.CancelURL, &p.CreatedAt, &p.EventPublished)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrPaymentNotFound
	}
	if err != nil {
		return p, fmt.Errorf("read payment: %w", err)
	}
	return p, nil
}

func (r *PostgresPaymentRepository) FindByOrder(ctx context.Context, orderID string) (domain.Payment, error) {
	return scanPayment(r.pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE order_id = $1`, orderID))
}
func (r *PostgresPaymentRepository) FindByID(ctx context.Context, id string) (domain.Payment, error) {
	return scanPayment(r.pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id))
}
func (r *PostgresPaymentRepository) CreateOrGet(ctx context.Context, p domain.Payment) (domain.Payment, error) {
	if p.Status == "" {
		p.Status = domain.PaymentPending
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO payments
 (id,order_id,customer_id,reservation_id,reservation_expires_at,amount_usd_cents,currency,provider_idempotency_key,success_url,cancel_url,status)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (order_id) DO NOTHING`, p.ID, p.OrderID, p.CustomerID, p.ReservationID,
		p.ReservationExpiresAt, p.AmountUSDCents, p.Currency, p.ProviderIdempotencyKey, p.SuccessURL, p.CancelURL, string(p.Status))
	if err != nil {
		return domain.Payment{}, fmt.Errorf("save payment: %w", err)
	}
	return r.FindByOrder(ctx, p.OrderID)
}
func (r *PostgresPaymentRepository) SaveSession(ctx context.Context, id string, s port.CheckoutSession) (domain.Payment, error) {
	_, err := r.pool.Exec(ctx, `UPDATE payments SET
 provider_session_id=$2, provider_payment_intent_id=NULLIF($3,''), payment_url=NULLIF($4,''), session_expires_at=$5,
 event_published=CASE WHEN status<>$6 AND $6 IN ('PAID','FAILED') THEN FALSE ELSE event_published END,
 retry_after=CASE WHEN status<>$6 AND $6 IN ('PAID','FAILED') THEN clock_timestamp() ELSE retry_after END,
 status=$6, updated_at=clock_timestamp()
 WHERE id=$1 AND (provider_session_id IS NULL OR provider_session_id=$2)
 AND (status='PENDING' OR $6='PAID' OR status=$6)`, id, s.SessionID, s.PaymentIntentID, s.PaymentURL, s.ExpiresAt, string(s.Status))
	if err != nil {
		return domain.Payment{}, fmt.Errorf("save provider session: %w", err)
	}
	return r.FindByID(ctx, id)
}
func (r *PostgresPaymentRepository) PendingEvents(ctx context.Context, limit int) ([]domain.Payment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+paymentColumns+` FROM payments
 WHERE status IN ('PAID','FAILED') AND event_published=FALSE AND COALESCE(retry_after,created_at)<=clock_timestamp()
 ORDER BY COALESCE(retry_after,created_at),id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Payment, 0)
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (r *PostgresPaymentRepository) MarkPublished(ctx context.Context, id string, status domain.PaymentStatus) error {
	// A later PAID result must still be published if an earlier FAILED event
	// was in flight. Only mark the exact state confirmed by the broker.
	_, err := r.pool.Exec(ctx, `UPDATE payments SET event_published=TRUE,retry_after=NULL,last_error=NULL
 WHERE id=$1 AND status=$2`, id, string(status))
	return err
}
func (r *PostgresPaymentRepository) RetryPublish(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE payments SET retry_after=clock_timestamp()+INTERVAL '10 seconds'
 WHERE id=$1 AND event_published=FALSE`, id)
	return err
}

package repository

import (
	"context"

	"ecommerce.local/payment-service/internal/port"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresWebhookRepository struct{ pool *pgxpool.Pool }

func NewPostgresWebhookRepository(pool *pgxpool.Pool) port.WebhookRepository {
	return &PostgresWebhookRepository{pool: pool}
}
func (r *PostgresWebhookRepository) Save(ctx context.Context, e port.VerifiedWebhook, payload []byte) (port.StoredWebhook, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO webhook_events(event_id,event_type,livemode,payload,session_id,status,processed_at)
 VALUES($1,$2,FALSE,$3::jsonb,NULLIF($4,''),CASE WHEN $5 THEN 'PENDING' ELSE 'IGNORED' END,
 CASE WHEN $5 THEN NULL ELSE clock_timestamp() END) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.EventType, payload, e.SessionID, e.Supported)
	if err != nil {
		return port.StoredWebhook{}, err
	}
	var stored port.StoredWebhook
	err = r.pool.QueryRow(ctx, `SELECT event_id,COALESCE(session_id,''),status<>'PENDING' FROM webhook_events WHERE event_id=$1`, e.EventID).
		Scan(&stored.EventID, &stored.SessionID, &stored.Processed)
	return stored, err
}
func (r *PostgresWebhookRepository) MarkProcessed(ctx context.Context, eventID, paymentID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE webhook_events SET status='PROCESSED',payment_id=$2,processed_at=clock_timestamp(),last_error=NULL
 WHERE event_id=$1 AND status='PENDING'`, eventID, paymentID)
	return err
}
func (r *PostgresWebhookRepository) Pending(ctx context.Context, limit int) ([]port.StoredWebhook, error) {
	rows, err := r.pool.Query(ctx, `SELECT event_id,COALESCE(session_id,'') FROM webhook_events
 WHERE status='PENDING' AND COALESCE(retry_after,received_at)<=clock_timestamp() ORDER BY received_at,event_id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]port.StoredWebhook, 0)
	for rows.Next() {
		var e port.StoredWebhook
		if err := rows.Scan(&e.EventID, &e.SessionID); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (r *PostgresWebhookRepository) Retry(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE webhook_events SET retry_after=clock_timestamp()+INTERVAL '10 seconds' WHERE event_id=$1 AND status='PENDING'`, id)
	return err
}

func (r *PostgresWebhookRepository) MarkIgnored(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE webhook_events SET status='IGNORED',processed_at=clock_timestamp()
 WHERE event_id=$1 AND status='PENDING'`, id)
	return err
}

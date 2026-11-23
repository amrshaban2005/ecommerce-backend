CREATE TABLE payments (
 id UUID PRIMARY KEY,
 order_id UUID NOT NULL UNIQUE,
 customer_id UUID NOT NULL,
 reservation_id UUID NOT NULL,
 reservation_expires_at TIMESTAMPTZ NOT NULL,
 amount_usd_cents BIGINT NOT NULL CHECK (amount_usd_cents > 0),
 currency TEXT NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PAID','FAILED')),
 provider_idempotency_key TEXT NOT NULL UNIQUE CHECK (length(btrim(provider_idempotency_key)) > 0),
 provider_session_id TEXT UNIQUE,
 provider_payment_intent_id TEXT UNIQUE,
 payment_url TEXT,
 session_expires_at TIMESTAMPTZ,
 success_url TEXT NOT NULL,
 cancel_url TEXT NOT NULL,
 event_published BOOLEAN NOT NULL DEFAULT FALSE,
 retry_after TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
 last_error TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX payments_publication_idx ON payments(retry_after)
 WHERE status IN ('PAID','FAILED') AND event_published=FALSE;

CREATE TABLE webhook_events (
 event_id TEXT PRIMARY KEY CHECK (length(btrim(event_id)) > 0),
 event_type TEXT NOT NULL CHECK (length(btrim(event_type)) > 0),
 payment_id UUID REFERENCES payments(id),
 session_id TEXT,
 livemode BOOLEAN NOT NULL CHECK (livemode=FALSE),
 payload JSONB NOT NULL CHECK (jsonb_typeof(payload)='object'),
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PROCESSED','IGNORED')),
 retry_after TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
 last_error TEXT,
 received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 processed_at TIMESTAMPTZ,
 CHECK ((status IN ('PROCESSED','IGNORED'))=(processed_at IS NOT NULL))
);
CREATE INDEX webhook_events_pending_idx ON webhook_events(retry_after) WHERE status='PENDING';

CREATE TABLE orders (
 id UUID PRIMARY KEY,
 customer_id UUID NOT NULL,
 status TEXT NOT NULL DEFAULT 'PENDING'
  CHECK (status IN ('PENDING', 'AWAITING_PAYMENT', 'CHECKOUT_FAILED', 'CLEANUP_PENDING', 'PAID', 'COMPLETED', 'EXPIRED')),
 currency TEXT NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
 total_usd NUMERIC NOT NULL CHECK (total_usd > 0 AND total_usd < 'Infinity'::numeric),
 reservation_id UUID UNIQUE,
 reservation_expires_at TIMESTAMPTZ,
 idempotency_key TEXT,
 request_hash TEXT,
 failure_code TEXT,
 last_error TEXT,
 retry_after TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE (customer_id, idempotency_key)
);
CREATE INDEX orders_customer_id_idx ON orders(customer_id);
CREATE INDEX orders_recovery_idx ON orders(status, retry_after)
 WHERE status IN ('PENDING', 'CLEANUP_PENDING');

CREATE TABLE order_items (
 order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
 product_id UUID NOT NULL,
 product_name TEXT NOT NULL,
 quantity INTEGER NOT NULL CHECK (quantity > 0),
 unit_price_usd NUMERIC NOT NULL CHECK (unit_price_usd > 0 AND unit_price_usd < 'Infinity'::numeric),
 PRIMARY KEY (order_id, product_id)
);

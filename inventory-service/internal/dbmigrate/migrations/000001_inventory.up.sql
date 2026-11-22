CREATE TABLE inventory (
 product_id UUID PRIMARY KEY,
 on_hand INTEGER NOT NULL CHECK (on_hand >= 0),
 reserved INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0 AND reserved <= on_hand)
);

CREATE TABLE reservations (
 id UUID PRIMARY KEY,
 order_id UUID NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'RELEASED')),
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 released_at TIMESTAMPTZ,
 CHECK ((status = 'RELEASED') = (released_at IS NOT NULL))
);
CREATE INDEX reservations_expiry_idx ON reservations(expires_at) WHERE status = 'ACTIVE';

CREATE TABLE reservation_items (
 reservation_id UUID NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
 product_id UUID NOT NULL REFERENCES inventory(product_id),
 quantity INTEGER NOT NULL CHECK (quantity > 0),
 PRIMARY KEY (reservation_id, product_id)
);

CREATE TABLE carts (
 id UUID PRIMARY KEY,
 customer_id UUID NOT NULL UNIQUE
);

CREATE TABLE cart_items (
 cart_id UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
 product_id UUID NOT NULL,
 quantity INTEGER NOT NULL CHECK (quantity > 0),
 PRIMARY KEY (cart_id, product_id)
);

-- A receipt makes retried purchased-quantity removal idempotent per order.
CREATE TABLE processed_orders (
 order_id UUID PRIMARY KEY,
 customer_id UUID NOT NULL,
 items_hash TEXT NOT NULL,
 processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

#!/usr/bin/env bash
# Local Docker Compose demo only. Set on-hand stock to 1000, keeping reservations.
set -euo pipefail

cd "$(dirname "$0")/.."

product_ids=$(mktemp)
trap 'rm -f "$product_ids"' EXIT

# Products and inventory live in different databases. Copy all catalog IDs first.
docker compose -f deploy/compose.infra.yml exec -T postgres \
  psql -X -U postgres -d product_db -v ON_ERROR_STOP=1 -At \
  -c 'SELECT id FROM products;' > "$product_ids"

# One transaction: update existing stock and create missing inventory records.
# If any item has more than 1000 reserved, the constraint rolls everything back.
{
  cat <<'SQL'
BEGIN;
CREATE TEMP TABLE demo_product_ids (product_id UUID PRIMARY KEY) ON COMMIT DROP;
COPY demo_product_ids (product_id) FROM STDIN;
SQL
  cat "$product_ids"
  printf '%s\n' '\.'
  cat <<'SQL'
UPDATE inventory SET on_hand = 1000;
INSERT INTO inventory (product_id, on_hand, reserved)
SELECT product_id, 1000, 0 FROM demo_product_ids
ON CONFLICT (product_id) DO UPDATE SET on_hand = 1000;
COMMIT;
SELECT product_id, on_hand, reserved, on_hand - reserved AS available
FROM inventory ORDER BY product_id;
SQL
} | docker compose -f deploy/compose.infra.yml exec -T postgres \
  psql -X -U postgres -d inventory_db -v ON_ERROR_STOP=1

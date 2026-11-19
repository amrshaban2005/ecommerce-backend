#!/bin/sh
set -eu

# Local services use PostgreSQL and Redis started by `make infra-up`.
# Run each service in a separate terminal. Environment variables override defaults.
# Add new service commands and configuration in the case block below.
SERVICE_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

case "${1:-}" in
    api-gateway)
        cd "$SERVICE_ROOT/api-gateway"
        exec env \
            HTTP_PORT="${HTTP_PORT:-8080}" \
            REDIS_HOST="${REDIS_HOST:-127.0.0.1}" \
            REDIS_PORT="${REDIS_PORT:-6379}" \
            RATE_LIMIT_PER_IP="${RATE_LIMIT_PER_IP:-100}" \
            MAX_REQUEST_BODY_BYTES="${MAX_REQUEST_BODY_BYTES:-1048576}" \
            UPSTREAM_TIMEOUT_SECONDS="${UPSTREAM_TIMEOUT_SECONDS:-30}" \
            AUTH_HTTP_ADDRESS="${AUTH_HTTP_ADDRESS:-http://127.0.0.1:8081}" \
            AUTH_GRPC_ADDRESS="${AUTH_GRPC_ADDRESS:-127.0.0.1:9090}" \
            AUTH_VERIFY_TIMEOUT_SECONDS="${AUTH_VERIFY_TIMEOUT_SECONDS:-1}" \
            PRODUCT_HTTP_ADDRESS="${PRODUCT_HTTP_ADDRESS:-http://127.0.0.1:8082}" \
            CART_HTTP_ADDRESS="${CART_HTTP_ADDRESS:-http://127.0.0.1:8083}" \
            INVENTORY_HTTP_ADDRESS="${INVENTORY_HTTP_ADDRESS:-http://127.0.0.1:8084}" \
            ORDER_HTTP_ADDRESS="${ORDER_HTTP_ADDRESS:-http://127.0.0.1:8085}" \
            PAYMENT_HTTP_ADDRESS="${PAYMENT_HTTP_ADDRESS:-http://127.0.0.1:8086}" \
            go run ./cmd/api-gateway
        ;;
    auth-service)
        : "${JWT_SECRET:?Export JWT_SECRET with at least 32 bytes before running auth-service}"
        cd "$SERVICE_ROOT/auth-service"
        exec env \
            HTTP_PORT="${HTTP_PORT:-8081}" \
            GRPC_PORT="${GRPC_PORT:-9090}" \
            POSTGRES_DSN="${POSTGRES_DSN:-postgres://auth:auth_local_only@127.0.0.1:5432/auth_db?sslmode=disable}" \
            JWT_SECRET="$JWT_SECRET" \
            go run ./cmd/auth-service
        ;;
    product-service)
        cd "$SERVICE_ROOT/product-service"
        exec env \
            HTTP_PORT="${HTTP_PORT:-8082}" \
            GRPC_PORT="${GRPC_PORT:-9091}" \
            POSTGRES_DSN="${POSTGRES_DSN:-postgres://product:product_local_only@127.0.0.1:5432/product_db?sslmode=disable}" \
            go run ./cmd/product-service
        ;;
    cart-service)
        cd "$SERVICE_ROOT/cart-service"
        exec env \
            HTTP_PORT="${HTTP_PORT:-8083}" \
            GRPC_PORT="${GRPC_PORT:-9092}" \
            POSTGRES_DSN="${POSTGRES_DSN:-postgres://cart:cart_local_only@127.0.0.1:5432/cart_db?sslmode=disable}" \
            go run ./cmd/cart-service
        ;;
    -h|--help)
        echo "Usage: $0 {api-gateway|auth-service|product-service|cart-service}"
        ;;
    *)
        echo "Usage: $0 {api-gateway|auth-service|product-service|cart-service}" >&2
        exit 1
        ;;
esac

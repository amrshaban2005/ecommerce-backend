# API gateway

Gateway foundation copied from the Go service template. Module: `ecommerce.local/api-gateway`.

The gateway currently runs an HTTP server using Gorilla Mux. It includes JSON logging, Gorilla proxy-header and recovery middleware, HTTP health checks, Prometheus metrics, and signal-driven graceful shutdown. It has no database or migrations and runs no gRPC server.

## Configuration

`HTTP_PORT` is required and must be an integer from 1 to 65535. Service HTTP addresses use environment values when set, otherwise they fall back to localhost ports 8081–8086 for Auth, Product, Cart, Inventory, Order and Payment respectively. There is no `.env` loading.

## Routes

| Route | Behavior |
| --- | --- |
| `GET /health/liveness` | Returns status UP |
| `GET /health/readiness` | Uses the lifecycle readiness flag |
| `/metrics` | Prometheus metrics |

## Proxy routing

The `Config.Routes()` function defines the path prefixes in code, using these environment variables for upstream addresses:

| Path prefix | Environment variable |
| --- | --- |
| `/api/v1/auth` | `AUTH_HTTP_ADDRESS` |
| `/api/v1/products` | `PRODUCT_HTTP_ADDRESS` |
| `/api/v1/admin/products` | `PRODUCT_HTTP_ADDRESS` |
| `/api/v1/cart` | `CART_HTTP_ADDRESS` |
| `/api/v1/admin/inventory` | `INVENTORY_HTTP_ADDRESS` |
| `/api/v1/checkout` | `ORDER_HTTP_ADDRESS` |
| `/api/v1/orders` | `ORDER_HTTP_ADDRESS` |
| `/api/v1/webhooks/stripe` | `PAYMENT_HTTP_ADDRESS` |

Example environment values:

```text
HTTP_PORT=8080
AUTH_HTTP_ADDRESS=http://auth-service:8080
PRODUCT_HTTP_ADDRESS=http://product-service:8080
CART_HTTP_ADDRESS=http://cart-service:8080
INVENTORY_HTTP_ADDRESS=http://inventory-service:8080
ORDER_HTTP_ADDRESS=http://order-service:8080
PAYMENT_HTTP_ADDRESS=http://payment-service:8080
```

The longest matching path prefix wins. Prefixes respect path boundaries: `/api/v1/products` does not match `/api/v1/products-other`. Operational health and metrics routes are registered before the proxy. The proxy preserves the request path, query, method, body and end-to-end headers, including Authorization. An upstream URL path is prepended using Go's reverse proxy behavior; use origin-only upstream URLs to preserve paths exactly. Hop-by-hop headers are handled by Go's reverse proxy.

Unmatched paths return JSON 404; upstream connection failures return JSON 502. Upstream response status, headers and body are forwarded. Invalid upstream URLs fail startup. Rate limiting, authentication and tenant behavior have not been added.

## Circuit breaker

Each route prefix has its own circuit breaker. Five consecutive upstream 5xx responses or connection failures open the circuit. Requests are then rejected with JSON 503. After ten seconds, one recovery probe is allowed: a non-5xx response closes the circuit, while another failure reopens it. Non-5xx responses reset the consecutive failure counter. The implementation also provides a recovery-window option, matching the reference gateway.

## Commands

From this folder, run `make test`, `make vet`, or `make build`. Build output is `bin/api`. `make run` starts the gateway with the required environment variables above.

The Dockerfile builds the binary and runs it as a non-root user. Build with this folder as context: `docker build -t api-gateway .`. Set `HTTP_PORT=8080` when using the exposed port 8080, and override the service address variables with reachable HTTP/HTTPS URLs for container deployments.

## Layout

`cmd/api-gateway/main.go` handles startup and signals. `internal/bootstrap/app.go` constructs, runs and shuts down the application. `internal/config` loads configuration. `internal/health` implements HTTP probes. `internal/proxy` implements upstream routing. `internal/circuit` implements circuit breaking. The adapter, application, domain and port folders retain the template's package declarations as placeholders.

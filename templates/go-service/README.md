# Go service template

Go module `template.local/service`, using Go 1.26.3.

## Structure

```text
cmd/api/main.go              Entry point, logging and signal handling
internal/
├── adapter/
│   ├── http/                Package declaration only
│   ├── grpc/                Package declaration only
│   └── repository/          Package declaration only
├── application/             Package declaration only
├── domain/                  Package declaration only
├── port/                    Package declaration only
├── bootstrap/app.go         Application creation, Run and Shutdown
├── config/                  Environment loading and validation
├── dbmigrate/               Migration runner and embedded SQL files
└── health/health.go         HTTP liveness/readiness handlers
```

## Configuration

The application reads these required environment variables:

| Variable | Value |
| --- | --- |
| `HTTP_PORT` | HTTP port, integer from 1 to 65535 |
| `GRPC_PORT` | gRPC port, integer from 1 to 65535 |
| `POSTGRES_DSN` | PostgreSQL connection string |

Configuration errors are reported together. There are no default values or `.env` loading. PostgreSQL must be reachable at startup.

## Runtime

Application creation opens and checks the PostgreSQL pool, applies pending migrations and constructs the servers. HTTP routing uses Gorilla Mux with Gorilla's `ProxyHeaders` and `RecoveryHandler` middleware.

| HTTP route | Behavior |
| --- | --- |
| `GET /health/liveness` | Returns HTTP 200 with status `UP` |
| `GET /health/readiness` | Returns HTTP 200 when the ready flag is true, otherwise HTTP 503 |
| `/metrics` | Prometheus metrics handler |

Readiness uses a lifecycle flag, not an ongoing database check. The gRPC server starts without registered services. The HTTP server has a five-second header-read timeout.

On SIGINT, SIGTERM or a server error, shutdown marks the application unready, gracefully stops gRPC, shuts down HTTP with a 15-second timeout, then closes the PostgreSQL pool after successful HTTP shutdown.

## Migrations

`internal/dbmigrate` uses `golang-migrate` with embedded, numbered `.up.sql` and `.down.sql` files. Startup applies pending up migrations and stops if migration fails. The baseline files contain only `SELECT 1`; they create no business tables.

## Commands

From this folder:

```sh
make test
make vet
make build
make run
```

`make build` writes `bin/api`. `make run` requires the environment variables listed above. `make fmt` formats Go files.

The Dockerfile builds the API binary and runs it as a non-root user. Its build context is this template folder:

```sh
docker build -t go-service-template .
```

Supply the required environment variables when running the container. Its Dockerfile exposes ports 8080 and 9090; set `HTTP_PORT` and `GRPC_PORT` accordingly when using those ports.

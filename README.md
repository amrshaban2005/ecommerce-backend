# Backend

```text
backend/
├── api-gateway/
├── templates/go-service/
├── go.work
├── go.work.sum
├── Makefile
└── README.md
```

- [API gateway](api-gateway/README.md): HTTP gateway with configurable reverse proxy routes, health checks, metrics and graceful shutdown.
- [Go service template](templates/go-service/README.md): reusable service foundation.

The workspace includes both modules. From this folder, `make test`, `make vet`, and `make build` check both. The gateway includes per-route circuit breaking. Rate limiting has not been added yet.

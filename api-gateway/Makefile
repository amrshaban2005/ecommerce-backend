.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/api ./cmd/api-gateway
run:
	go run ./cmd/api-gateway
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

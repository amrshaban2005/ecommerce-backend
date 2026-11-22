.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/order-service ./cmd/order-service
run:
	go run ./cmd/order-service
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

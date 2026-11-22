.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/inventory-service ./cmd/inventory-service
run:
	go run ./cmd/inventory-service
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

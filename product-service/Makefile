.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/product-service ./cmd/product-service
run:
	go run ./cmd/product-service
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

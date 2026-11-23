.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/payment-service ./cmd/payment-service
run:
	go run ./cmd/payment-service
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

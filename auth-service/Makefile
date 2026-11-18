.PHONY: test build run fmt vet

test:
	GOWORK=off go test -race ./...
build:
	GOWORK=off go build -o bin/auth-service ./cmd/auth-service
run:
	go run ./cmd/auth-service
fmt:
	gofmt -w .
vet:
	GOWORK=off go vet ./...

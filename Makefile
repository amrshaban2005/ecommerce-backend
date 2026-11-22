.PHONY: test build vet

test:
	$(MAKE) -C cart-service test
	$(MAKE) -C product-service test
	$(MAKE) -C auth-service test
	$(MAKE) -C api-gateway test
	$(MAKE) -C templates/go-service test
build:
	$(MAKE) -C order-service build
	$(MAKE) -C inventory-service build
	$(MAKE) -C cart-service build
	$(MAKE) -C product-service build
	cd contracts && GOWORK=off go build ./...
	$(MAKE) -C auth-service build
	$(MAKE) -C api-gateway build
	$(MAKE) -C templates/go-service build
vet:
	$(MAKE) -C order-service vet
	$(MAKE) -C inventory-service vet
	$(MAKE) -C cart-service vet
	$(MAKE) -C product-service vet
	cd contracts && GOWORK=off go vet ./...
	$(MAKE) -C auth-service vet
	$(MAKE) -C api-gateway vet
	$(MAKE) -C templates/go-service vet

COMPOSE_INFRA = docker compose -f deploy/compose.infra.yml
COMPOSE_SERVICES = docker compose -f deploy/compose.services.yml

.PHONY: infra-up services-up services-down infra-down down

infra-up:
	$(COMPOSE_INFRA) up -d --wait

services-up: infra-up
	$(COMPOSE_SERVICES) up -d --build --wait

services-down:
	$(COMPOSE_SERVICES) down

infra-down: services-down
	$(COMPOSE_INFRA) down

down: infra-down

.PHONY: proto

proto:
	protoc -I contracts/proto --go_out=contracts --go_opt=paths=source_relative --go-grpc_out=contracts --go-grpc_opt=paths=source_relative $(wildcard contracts/proto/*/v1/*.proto)

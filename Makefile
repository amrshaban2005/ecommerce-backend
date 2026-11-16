.PHONY: test build vet

test:
	$(MAKE) -C api-gateway test
	$(MAKE) -C templates/go-service test
build:
	$(MAKE) -C api-gateway build
	$(MAKE) -C templates/go-service build
vet:
	$(MAKE) -C api-gateway vet
	$(MAKE) -C templates/go-service vet

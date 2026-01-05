.PHONY: build run test lint clean docker-build docker-push

# Variables
BINARY_NAME=internal-gateway
DOCKER_IMAGE=sandbox0/internal-gateway
VERSION?=latest

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build the binary
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) -v ./cmd/internal-gateway

# Run locally
run: build
	./bin/$(BINARY_NAME)

# Run tests
test:
	$(GOTEST) -v -race -cover ./...

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	$(GOFMT) -s -w .

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf vendor/

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Vendor dependencies
vendor:
	$(GOMOD) vendor

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

# Push Docker image
docker-push:
	docker push $(DOCKER_IMAGE):$(VERSION)

# Build and push Docker image
docker: docker-build docker-push

# Generate mocks (if needed)
mocks:
	mockgen -source=pkg/db/repository.go -destination=pkg/db/mocks/repository_mock.go

# Run locally with environment
run-local:
	DATABASE_URL="postgres://localhost:5432/sandbox0?sslmode=disable" \
	MANAGER_URL="http://localhost:8080" \
	STORAGE_PROXY_URL="http://localhost:8081" \
	GATEWAY_HTTP_PORT=8443 \
	./bin/$(BINARY_NAME)

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  run          - Build and run locally"
	@echo "  test         - Run tests"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Download dependencies"
	@echo "  vendor       - Vendor dependencies"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-push  - Push Docker image"
	@echo "  docker       - Build and push Docker image"
	@echo "  run-local    - Run locally with environment"


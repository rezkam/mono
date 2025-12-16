# Binary name
BINARY_NAME=mono-server
# Docker image name
DOCKER_IMAGE=mono-service

.PHONY: all help gen tidy test build run clean docker-build docker-run

# Default target
all: gen tidy security test build

# Help target to document all commands
help: ## Display this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

gen: ## Generate Go code from Protobuf files using buf
	@echo "Generating Protobuf files..."
	export PATH=$(PATH):$(HOME)/go/bin && buf generate api/proto

security: ## Run govulncheck to find vulnerabilities
	@echo "Checking for vulnerabilities..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

tidy: ## Run go mod tidy to update dependencies
	@echo "Tidying module dependencies..."
	go mod tidy

test: ## Run unit tests
	@echo "Running tests..."
	go test -v ./...

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	go test ./... -bench=. -benchmem

test-gcs: ## Run GCS integration tests (requires TEST_GCS_BUCKET and GOOGLE_APPLICATION_CREDENTIALS)
	@echo "Running GCS integration tests..."
	@if [ -z "$(TEST_GCS_BUCKET)" ]; then \
		echo "Error: TEST_GCS_BUCKET environment variable not set"; \
		echo "Usage: TEST_GCS_BUCKET=your-bucket-name make test-gcs"; \
		exit 1; \
	fi
	TEST_GCS_BUCKET=$(TEST_GCS_BUCKET) go test -v ./internal/storage/gcs -run TestGCSStore

build: ## Build the binary locally
	@echo "Building binary..."
	go build -o $(BINARY_NAME) cmd/server/main.go

run: build ## Build and run the binary locally with default settings (FS storage, OTel disabled)
	@echo "Running server..."
	MONO_STORAGE_TYPE=fs MONO_FS_DIR=./mono-data MONO_GRPC_PORT=8080 MONO_HTTP_PORT=8081 MONO_OTEL_ENABLED=false ./$(BINARY_NAME)

clean: ## Remove built binary and temporary data
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -rf mono-data

docker-build: ## Build the Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

docker-run: ## Run the Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 8081:8081 $(DOCKER_IMAGE)

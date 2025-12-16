# Binary name
BINARY_NAME=mono-server
# Docker image name
DOCKER_IMAGE=mono-service

.PHONY: all help gen gen-sqlc tidy test build run clean docker-build docker-run db-up db-down db-migrate-up db-migrate-down db-migrate-create test-sql

# Default target
all: gen tidy security test build

# Check if git hooks are configured
GIT_HOOKS_PATH := $(shell git config core.hooksPath)
EXPECTED_HOOKS_PATH := .githooks

ifneq ($(GIT_HOOKS_PATH),$(EXPECTED_HOOKS_PATH))
    $(warning "Git hooks are not configured to use $(EXPECTED_HOOKS_PATH). Run 'make setup-hooks' to configure them.")
endif

# Help target to document all commands
help: ## Display this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

gen: ## Generate Go code from Protobuf files using buf
	@echo "Generating Protobuf files..."
	export PATH=$(PATH):$(HOME)/go/bin && buf generate api/proto

gen-sqlc: ## Generate type-safe Go code from SQL queries using sqlc
	@echo "Generating sqlc code..."
	sqlc generate

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

lint: ## Run linter
	@echo "Running linter..."
	go vet ./...

setup-hooks: ## Configure git hooks to run automatically
	@git config core.hooksPath .githooks
	@echo "Git hooks configured! Hooks in .githooks/ will now run automatically."

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

db-up: ## Start PostgreSQL database using Docker Compose
	@echo "Starting PostgreSQL database..."
	docker compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3

db-down: ## Stop and remove database containers
	@echo "Stopping database containers..."
	docker compose down

db-migrate-up: ## Run database migrations up
	@echo "Running migrations up..."
	@if [ -z "$(DB_URL)" ]; then \
		echo "Usage: DB_URL='postgres://user:pass@localhost:5432/dbname' make db-migrate-up"; \
		echo "   or: DB_URL='./data.db' make db-migrate-up"; \
		exit 1; \
	fi
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/storage/sql/migrations $(DB_DRIVER) "$(DB_URL)" up

db-migrate-down: ## Rollback last database migration
	@echo "Rolling back migration..."
	@if [ -z "$(DB_URL)" ]; then \
		echo "Usage: DB_URL='postgres://user:pass@localhost:5432/dbname' make db-migrate-down"; \
		echo "   or: DB_URL='./data.db' make db-migrate-down"; \
		exit 1; \
	fi
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/storage/sql/migrations $(DB_DRIVER) "$(DB_URL)" down

db-migrate-create: ## Create a new migration file (usage: NAME=create_users make db-migrate-create)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required"; \
		echo "Usage: NAME=create_users make db-migrate-create"; \
		exit 1; \
	fi
	@echo "Creating new migration: $(NAME)"
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/storage/sql/migrations create $(NAME) sql

test-sql: ## Run SQL storage integration tests (requires running database)
	@echo "Running SQL integration tests..."
	go test -v ./internal/storage/sql/...

# Binary name
BINARY_NAME=mono-server
WORKER_BINARY_NAME=mono-worker
# Docker image name
DOCKER_IMAGE=mono-service
# Default DB Driver
DB_DRIVER ?= postgres

.PHONY: all help gen gen-sqlc tidy fmt fmt-check test build build-worker build-apikey gen-apikey run clean docker-build docker-run db-up db-down db-migrate-up db-migrate-down db-migrate-create test-sql test-integration test-integration-up test-integration-down test-integration-clean test-integration-full test-all test-db-status test-db-logs test-db-shell bench bench-test lint setup-hooks security

# Default target - show help when no target specified
all: help

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
	@PATH="$$PATH:$(HOME)/go/bin" buf generate api/proto

gen-sqlc: ## Generate type-safe Go code from SQL queries using sqlc
	@echo "Generating sqlc code..."
	sqlc generate
	@echo "Fixing sqlc imports (removing legacy pgtype)..."
	for file in internal/storage/sql/sqlcgen/*.go; do \
		sed -i.bak -e '/"github.com\/jackc\/pgtype"/d' "$$file"; \
		rm "$$file.bak"; \
	done

security: ## Run govulncheck to find vulnerabilities
	@echo "Checking for vulnerabilities..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

tidy: ## Run go mod tidy to update dependencies
	@echo "Tidying module dependencies..."
	go mod tidy

fmt: ## Format all Go files tracked in git
	@echo "Formatting Go files..."
	gofmt -w $$(git ls-files '*.go')

fmt-check: ## Ensure all Go files are gofmt'ed
	@echo "Checking Go formatting..."
	@if [ -n "$$(gofmt -l $$(git ls-files '*.go'))" ]; then \
		echo "The following files need gofmt (run 'make fmt' or 'gofmt -w'): "; \
		gofmt -l $$(git ls-files '*.go'); \
		exit 1; \
	fi

test: ## Run unit tests
	@echo "Running tests..."
	go test -v ./...

bench: ## Run benchmarks with real database (requires BENCHMARK_POSTGRES_URL)
	@echo "Running benchmarks..."
	@if [ -z "$(BENCHMARK_POSTGRES_URL)" ]; then \
		echo "Warning: BENCHMARK_POSTGRES_URL not set. Set it to run benchmarks with real database."; \
		echo "Usage: BENCHMARK_POSTGRES_URL='postgres://user:pass@localhost:5432/dbname' make bench"; \
		echo "Skipping benchmarks..."; \
	else \
		BENCHMARK_POSTGRES_URL=$(BENCHMARK_POSTGRES_URL) go test -bench=. -benchmem ./internal/service/...; \
	fi

bench-test: ## Run benchmarks using test database (cleans volumes before and after)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running benchmarks with test database ==="
	@BENCHMARK_POSTGRES_URL="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -bench=. -benchmem ./internal/service/...; \
	BENCH_RESULT=$$?; \
	echo ""; \
	echo "=== Cleaning up test database ==="; \
	$(MAKE) test-integration-clean; \
	echo ""; \
	if [ $$BENCH_RESULT -eq 0 ]; then \
		echo "✅ Benchmarks completed successfully"; \
	else \
		echo "❌ Benchmarks failed"; \
		exit $$BENCH_RESULT; \
	fi


build: ## Build the binary locally
	@echo "Building binary..."
	go build -o $(BINARY_NAME) cmd/server/main.go

build-worker: ## Build the recurring worker binary
	@echo "Building worker..."
	go build -o $(WORKER_BINARY_NAME) cmd/worker/main.go

build-apikey: ## Build the API key generator
	@echo "Building API key generator..."
	go build -o mono-apikey cmd/apikey/main.go

gen-apikey: build-apikey ## Generate a new API key (usage: NAME="My Key" DAYS=30 make gen-apikey)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required"; \
		echo "Usage: NAME=\"My Key\" make gen-apikey"; \
		echo "       NAME=\"My Key\" DAYS=30 make gen-apikey (with expiration)"; \
		exit 1; \
	fi
	@if [ -z "$(POSTGRES_URL)" ] && [ ! -f .db.env ]; then \
		echo "Error: POSTGRES_URL environment variable is required"; \
		echo "Tip: Run 'make db-up' first to start the database and set POSTGRES_URL automatically"; \
		exit 1; \
	fi
	@if [ -z "$(DAYS)" ]; then \
		if [ -f .db.env ]; then . ./.db.env && ./mono-apikey -name "$(NAME)"; else ./mono-apikey -name "$(NAME)"; fi; \
	else \
		if [ -f .db.env ]; then . ./.db.env && ./mono-apikey -name "$(NAME)" -days $(DAYS); else ./mono-apikey -name "$(NAME)" -days $(DAYS); fi; \
	fi

lint: ## Run linter
	@echo "Running linter..."
	go vet ./...

setup-hooks: ## Configure git hooks to run automatically
	@git config core.hooksPath .githooks
	@echo "Git hooks configured! Hooks in .githooks/ will now run automatically."

run: build ## Build and run the binary locally (requires MONO_POSTGRES_URL)
	@echo "Running server..."
	@if [ -z "$(MONO_POSTGRES_URL)" ]; then \
		echo "Error: MONO_POSTGRES_URL environment variable is required"; \
		echo "Usage: MONO_POSTGRES_URL='postgres://user:pass@localhost:5432/dbname' make run"; \
		exit 1; \
	fi
	MONO_STORAGE_TYPE=postgres MONO_GRPC_PORT=8080 MONO_HTTP_PORT=8081 MONO_OTEL_ENABLED=false ./$(BINARY_NAME)

clean: ## Remove built binaries
	@echo "Cleaning up..."
	@rm -f mono-server mono-worker mono-apikey
	@echo "All binaries removed"

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
	@echo 'export POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db?sslmode=disable"' > .db.env
	@echo "✅ Database URL saved to .db.env"

db-down: ## Stop and remove database containers
	@echo "Stopping database containers..."
	docker compose down
	@rm -f .db.env
	@echo "Database environment cleaned up"

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

# Integration test targets with database lifecycle
.PHONY: test-integration test-integration-up test-integration-down test-integration-clean test-integration-full

test-integration-up: ## Start PostgreSQL test database
	@echo "Starting PostgreSQL test database..."
	docker compose -f docker-compose.test.yml up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then \
			echo "PostgreSQL is ready!"; \
			break; \
		fi; \
		echo "Waiting for PostgreSQL... ($$i/10)"; \
		sleep 2; \
	done
	@echo "Running migrations..."
	@TEST_POSTGRES_URL="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go run github.com/pressly/goose/v3/cmd/goose@latest \
		-dir internal/storage/sql/migrations \
		postgres \
		"postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		up

test-integration-down: ## Stop PostgreSQL test database
	@echo "Stopping PostgreSQL test database..."
	docker compose -f docker-compose.test.yml down

test-integration-clean: ## Stop and remove PostgreSQL test database with volumes
	@echo "Cleaning up PostgreSQL test database and volumes..."
	docker compose -f docker-compose.test.yml down -v
	@echo "Cleanup complete!"

test-integration: ## Run integration tests with clean database (removes volumes before and after)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running integration tests ==="
	@TEST_POSTGRES_URL="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -v ./tests/integration/... -count=1; \
	TEST_RESULT=$$?; \
	echo ""; \
	echo "=== Cleaning up test database ==="; \
	$(MAKE) test-integration-clean; \
	echo ""; \
	if [ $$TEST_RESULT -eq 0 ]; then \
		echo "✅ Integration tests PASSED"; \
	else \
		echo "❌ Integration tests FAILED"; \
		exit $$TEST_RESULT; \
	fi

test-integration-full: ## Complete integration test cycle: start DB, run migrations, test, cleanup
	@echo "=== Starting full integration test cycle ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running integration tests ==="
	@TEST_POSTGRES_URL="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -v ./tests/integration/... -count=1; \
	TEST_RESULT=$$?; \
	echo ""; \
	echo "=== Cleaning up test database ==="; \
	$(MAKE) test-integration-clean; \
	echo ""; \
	if [ $$TEST_RESULT -eq 0 ]; then \
		echo "✅ Integration tests PASSED"; \
	else \
		echo "❌ Integration tests FAILED"; \
		exit $$TEST_RESULT; \
	fi

test-all: ## Run all tests (unit + integration)
	@echo "=== Running unit tests ==="
	@go test -v ./internal/recurring/... ./internal/service/...
	@echo ""
	@echo "=== Running integration tests ==="
	@$(MAKE) test-integration-full

# Helper targets
.PHONY: test-db-status test-db-logs test-db-shell

test-db-status: ## Check PostgreSQL test database status
	@docker compose -f docker-compose.test.yml ps

test-db-logs: ## Show PostgreSQL test database logs
	@docker compose -f docker-compose.test.yml logs -f postgres

test-db-shell: ## Connect to PostgreSQL test database shell
	@docker compose -f docker-compose.test.yml exec postgres psql -U postgres -d mono_test

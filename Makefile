# Binary name
BINARY_NAME=mono-server
WORKER_BINARY_NAME=mono-worker
# Docker image name
DOCKER_IMAGE=mono-service
# Default DB Driver
DB_DRIVER ?= postgres
# Development database DSN (used by db-up, run, gen-apikey, db-migrate-* targets)
DEV_STORAGE_DSN ?= postgres://mono:mono_password@localhost:5432/mono_db?sslmode=disable

# =============================================================================
# Database Architecture
# =============================================================================
# This project uses TWO separate PostgreSQL databases to isolate development
# from testing:
#
# 1. DEVELOPMENT DATABASE (docker-compose.yml)
#    - Port: 5432
#    - Container: mono-postgres
#    - Database: mono_db
#    - User: mono
#    - Commands: make db-up, make db-down
#    - Purpose: Local development, manual testing, persistent data
#
# 2. TEST DATABASE (docker-compose.test.yml)
#    - Port: 5433
#    - Container: mono-postgres-test
#    - Database: mono_test
#    - User: postgres
#    - Commands: make test-integration, make test-integration-up/down/clean
#    - Purpose: Automated tests, CI/CD, wiped between test runs
#
# Both databases can run simultaneously on different ports.
# =============================================================================

.PHONY: all help gen gen-openapi gen-sqlc tidy fmt fmt-check test build build-worker build-apikey gen-apikey run clean docker-build docker-run db-up db-down db-clean db-migrate-up db-migrate-down db-migrate-create test-sql test-integration test-integration-up test-integration-down test-integration-clean test-integration-http test-e2e test-all test-db-status test-db-logs test-db-shell bench bench-test lint build-timeutc-linter setup-hooks security sync-agents

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

gen-openapi: ## Generate Go code from OpenAPI spec
	@command -v oapi-codegen >/dev/null 2>&1 || { echo "Error: oapi-codegen is not installed. Install with: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest"; exit 1; }
	@echo "Generating OpenAPI code..."
	@oapi-codegen -config api/openapi/oapi-codegen.yaml api/openapi/mono.yaml

gen: gen-openapi gen-sqlc ## Generate all code (OpenAPI, sqlc)

gen-sqlc: ## Generate type-safe Go code from SQL queries using sqlc
	@command -v sqlc >/dev/null 2>&1 || { echo "Error: sqlc is not installed. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; exit 1; }
	@echo "Generating sqlc code..."
	sqlc generate
	@echo "Fixing sqlc imports (removing legacy pgtype)..."
	for file in internal/infrastructure/persistence/postgres/sqlcgen/*.go; do \
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
	@FILES=$$(git ls-files '*.go' 2>/dev/null | while read f; do [ -f "$$f" ] && echo "$$f"; done); \
	if [ -n "$$FILES" ]; then \
		echo "$$FILES" | xargs gofmt -w; \
	else \
		echo "No Go files to format"; \
	fi

fmt-check: ## Ensure all staged Go files are gofmt'ed (used by pre-commit hook)
	@echo "Checking Go formatting..."
	@STAGED_FILES=$$(git diff --cached --name-only --diff-filter=ACMR '*.go' 2>/dev/null || true); \
	if [ -z "$$STAGED_FILES" ]; then \
		echo "No staged Go files to check"; \
		exit 0; \
	fi; \
	UNFORMATTED=$$(echo "$$STAGED_FILES" | xargs gofmt -l 2>/dev/null || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following staged files need gofmt (run 'make fmt' or 'gofmt -w'):"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi

test: ## Run unit tests only (no database required)
	@echo "Running unit tests..."
	@go list ./... | grep -v '/tests/integration' | grep -v '/tests/e2e' | xargs go test -v

bench: ## Run benchmarks (requires MONO_STORAGE_DSN env var)
	@echo "Running benchmarks..."
	@if [ -z "$(MONO_STORAGE_DSN)" ]; then \
		echo "Warning: MONO_STORAGE_DSN not set. Set it to run benchmarks with real database."; \
		echo "Usage: MONO_STORAGE_DSN='postgres://user:pass@localhost:5432/dbname' make bench"; \
		echo "Skipping benchmarks..."; \
	else \
		MONO_STORAGE_DSN=$(MONO_STORAGE_DSN) go test -bench=. -benchmem ./tests/integration/postgres; \
	fi

bench-test: ## Run benchmarks using test database (port 5433, auto-cleanup)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running benchmarks with test database ==="
	@MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -bench=. -benchmem ./tests/integration/postgres; \
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


build: gen ## Build the server binary
	@echo "Building binary..."
	go build -o $(BINARY_NAME) ./cmd/server

build-worker: ## Build the background worker binary
	@echo "Building worker..."
	go build -o $(WORKER_BINARY_NAME) cmd/worker/main.go

build-apikey: ## Build the API key generator tool
	@echo "Building API key generator..."
	go build -o mono-apikey cmd/apikey/main.go

gen-apikey: build-apikey ## Generate a new API key (usage: NAME="My Key" DAYS=30 make gen-apikey)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required"; \
		echo "Usage: NAME=\"My Key\" make gen-apikey"; \
		echo "       NAME=\"My Key\" DAYS=30 make gen-apikey (with expiration)"; \
		exit 1; \
	fi
	@if [ -z "$(DAYS)" ]; then \
		MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" ./mono-apikey -name "$(NAME)"; \
	else \
		MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" ./mono-apikey -name "$(NAME)" -days $(DAYS); \
	fi

build-timeutc-linter: ## Build custom timezone linter
	@echo "Building timeutc linter..."
	@cd tools/linters/timeutc && go build -o ../../../timeutc ./cmd/timeutc

lint: build-timeutc-linter ## Run linters (golangci-lint + custom timezone linter)
	@echo "Verifying golangci-lint config..."
	golangci-lint config verify
	@echo "Running golangci-lint..."
	golangci-lint run
	@echo "Running custom timezone linter..."
	./timeutc ./...

setup-hooks: ## Configure git hooks to run automatically
	@git config core.hooksPath .githooks
	@echo "Git hooks configured! Hooks in .githooks/ will now run automatically."

run: build ## Build and run server using dev database
	@echo "Running server..."
	MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" MONO_REST_PORT=8081 MONO_OTEL_ENABLED=false ./$(BINARY_NAME)

clean: ## Remove built binaries
	@echo "Cleaning up..."
	@rm -f mono-server mono-worker mono-apikey timeutc
	@echo "All binaries removed"

docker-build: ## Build the Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

docker-run: ## Run the Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 8081:8081 $(DOCKER_IMAGE)

db-up: ## [DEV DB] Start development database (port 5432)
	@echo "Starting PostgreSQL database..."
	docker compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@echo "✅ Database ready at $(DEV_STORAGE_DSN)"

db-down: ## [DEV DB] Stop development database
	@echo "Stopping database containers..."
	docker compose down

db-clean: ## [DEV DB] Stop and remove development database with all data
	@echo "Cleaning up development database and volumes..."
	docker compose down -v
	@echo "Development database cleaned!"

db-migrate-up: ## Run migrations (usage: DB_URL=... make db-migrate-up)
	@echo "Running migrations up..."
	@if [ -z "$(DB_URL)" ]; then \
		echo "Usage: DB_URL='postgres://user:pass@localhost:5432/dbname' make db-migrate-up"; \
		echo "   or: DB_URL='./data.db' make db-migrate-up"; \
		exit 1; \
	fi
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations $(DB_DRIVER) "$(DB_URL)" up

db-migrate-down: ## Rollback migration (usage: DB_URL=... make db-migrate-down)
	@echo "Rolling back migration..."
	@if [ -z "$(DB_URL)" ]; then \
		echo "Usage: DB_URL='postgres://user:pass@localhost:5432/dbname' make db-migrate-down"; \
		echo "   or: DB_URL='./data.db' make db-migrate-down"; \
		exit 1; \
	fi
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations $(DB_DRIVER) "$(DB_URL)" down

db-migrate-create: ## Create migration (usage: NAME=create_users make db-migrate-create)
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is required"; \
		echo "Usage: NAME=create_users make db-migrate-create"; \
		exit 1; \
	fi
	@echo "Creating new migration: $(NAME)"
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations create $(NAME) sql

test-sql: ## Run SQL storage tests (requires running database)
	@echo "Running SQL integration tests..."
	go test -v ./internal/infrastructure/persistence/postgres/...

test-integration-up: ## [TEST DB] Start test database (port 5433)
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
	@MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go run github.com/pressly/goose/v3/cmd/goose@latest \
		-dir internal/infrastructure/persistence/postgres/migrations \
		postgres \
		"postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		up

test-integration-down: ## [TEST DB] Stop test database
	@echo "Stopping PostgreSQL test database..."
	docker compose -f docker-compose.test.yml down

test-integration-clean: ## [TEST DB] Stop test database (data is ephemeral via tmpfs)
	@echo "Stopping PostgreSQL test database..."
	docker compose -f docker-compose.test.yml down
	@echo "Test database stopped (data was in tmpfs, already gone)"

test-integration: ## [TEST DB] Run integration tests (auto-cleanup before/after)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running integration tests ==="
	@MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		$(MAKE) test-integration-run; \
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

test-integration-run: ## Run integration tests (requires MONO_STORAGE_DSN env var)
ifndef MONO_STORAGE_DSN
	$(error MONO_STORAGE_DSN is required. Set it to your PostgreSQL connection string.)
endif
	@# -count=1 disables test caching to ensure tests run fresh against real database
	@# -p 1 runs test packages sequentially (not in parallel) to avoid database conflicts
	go test -v -p 1 ./tests/integration/... -count=1

test-e2e: ## [TEST DB] Run end-to-end tests (auto-cleanup before/after)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running e2e tests ==="
	@MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -v ./tests/e2e -count=1; \
	TEST_RESULT=$$?; \
	echo ""; \
	echo "=== Cleaning up test database ==="; \
	$(MAKE) test-integration-clean; \
	echo ""; \
	if [ $$TEST_RESULT -eq 0 ]; then \
		echo "✅ E2E tests PASSED"; \
	else \
		echo "❌ E2E tests FAILED"; \
		exit $$TEST_RESULT; \
	fi

test-all: ## Run all tests (unit tests + integration tests + e2e tests)
	@echo "=== Running unit tests ==="
	@go test -v ./internal/recurring/...
	@echo ""
	@echo "=== Running integration tests (postgres) ==="
	@$(MAKE) test-integration
	@echo ""
	@echo "=== Running integration tests (http) ==="
	@$(MAKE) test-integration-http
	@echo ""
	@echo "=== Running e2e tests ==="
	@$(MAKE) test-e2e

test-integration-http: ## [TEST DB] Run HTTP integration tests (auto-cleanup before/after)
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running HTTP integration tests ==="
	@MONO_STORAGE_DSN="postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable" \
		go test -v ./tests/integration/http -count=1; \
	TEST_RESULT=$$?; \
	echo ""; \
	echo "=== Cleaning up test database ==="; \
	$(MAKE) test-integration-clean; \
	echo ""; \
	if [ $$TEST_RESULT -eq 0 ]; then \
		echo "✅ HTTP integration tests PASSED"; \
	else \
		echo "❌ HTTP integration tests FAILED"; \
		exit $$TEST_RESULT; \
	fi

# Helper targets
.PHONY: test-db-status test-db-logs test-db-shell

test-db-status: ## [TEST DB] Show test database container status
	@docker compose -f docker-compose.test.yml ps

test-db-logs: ## Show PostgreSQL test database logs
	@docker compose -f docker-compose.test.yml logs -f postgres

test-db-shell: ## Connect to PostgreSQL test database shell
	@docker compose -f docker-compose.test.yml exec postgres psql -U postgres -d mono_test

# =============================================================================
# Documentation Sync
# =============================================================================

sync-agents: ## Sync CLAUDE.md to AGENTS.md
	@echo "Syncing agent instruction files..."
	@cp CLAUDE.md AGENTS.md
	@echo "Synced CLAUDE.md to AGENTS.md"

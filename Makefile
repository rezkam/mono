# =============================================================================
# Mono Service Makefile
# Converted from justfile - use 'make help' to see available targets
# =============================================================================

# Use bash for all shell commands
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

# Binary names
BINARY_NAME := mono-server
WORKER_BINARY_NAME := mono-worker
DOCKER_IMAGE := mono-service

# Production worker instances (add worker-3, worker-4, etc. here)
WORKERS := worker-1 worker-2

# Migration image configuration
GOOSE_VERSION := v3.26.0
MIGRATE_IMAGE := ghcr.io/rezkam/goose-migrate
MIGRATE_IMAGE_TAG := $(GOOSE_VERSION)

# Default DB Driver
DB_DRIVER ?= postgres

# Development database DSN
DEV_STORAGE_DSN ?= postgres://mono:mono_password@localhost:5432/mono_db?sslmode=disable

# Test database DSN
TEST_DSN := postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable

# Git hooks configuration
EXPECTED_HOOKS_PATH := .githooks

# Color output
export FORCE_COLOR := 1

# Arguments for parameterized targets (set via command line: make test-one RUN=TestName)
RUN ?=
PKG ?= ./tests/integration/...
SERVICE ?=
NAME ?=
DAYS ?=
WORKER ?= worker-1
DB_URL ?=

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
#    - Purpose: Local automated tests, wiped between test runs
#
# Both databases can run simultaneously on different ports.
# =============================================================================

.PHONY: help default check gen-openapi gen-sqlc gen tidy fmt fmt-check fmt-check-all \
        security build-timeutc-linter build-nointerface-linter lint-interface lint-interface-fix \
        lint setup-hooks test test-race test-one bench bench-test build build-worker build-apikey \
        gen-apikey run clean docker-build docker-run docker-up docker-build-up docker-rebuild \
        docker-down docker-restart docker-restart-server docker-restart-workers docker-logs \
        docker-logs-server docker-logs-workers docker-logs-postgres docker-ps docker-clean \
        docker-shell-server docker-shell-worker docker-shell-postgres docker-health docker-health-server \
        docker-gen-apikey docker-buildx-setup docker-build-migrate docker-push-migrate \
        db-up db-down db-clean db-migrate-up db-migrate-down db-migrate-create \
        test-integration-up test-integration-down test-integration-clean test-integration-run \
        test-integration test-integration-http test-e2e test-sql test-all test-all-bench \
        test-db-status test-db-logs test-db-shell sync-agents \
        pgo-collect pgo-build pgo-clean \
        sandbox-build sandbox-run sandbox-run-secure sandbox-prompt sandbox-continue \
        sandbox-ls sandbox-inspect sandbox-rm sandbox-clean

# Default target (shows help)
.DEFAULT_GOAL := help

# Display this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "For targets requiring arguments, use: make target VAR=value"
	@echo "Example: make test-one RUN=TestName PKG=./tests/integration/..."

# Run everything: build, lint, and all tests (minimal output, fail-fast)
check: ## Run build, lint, and all tests with minimal output
	@printf "Building... "; OUTPUT=$$($(MAKE) -s build 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@printf "Linting... "; OUTPUT=$$($(MAKE) -s lint 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@printf "Unit tests... "; OUTPUT=$$(go list ./... | grep -v '/tests/' | xargs go test 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@printf "Integration (postgres)... "; OUTPUT=$$($(MAKE) -s test-integration 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@printf "Integration (http)... "; OUTPUT=$$($(MAKE) -s test-integration-http 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@if docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres >/dev/null 2>&1; then \
		printf "Security tests... "; OUTPUT=$$(MONO_STORAGE_DSN="$(TEST_DSN)" go test ./tests/security -count=1 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }; \
	fi
	@printf "E2E tests... "; OUTPUT=$$($(MAKE) -s test-e2e 2>&1) && echo "OK" || { echo "FAIL"; echo "$$OUTPUT"; exit 1; }
	@echo ""
	@echo "OK: All checks passed"

# Private target to check git hooks
_check-hooks:
	@HOOKS_PATH=$$(git config core.hooksPath || echo ""); \
	if [ "$$HOOKS_PATH" != "$(EXPECTED_HOOKS_PATH)" ]; then \
		echo "⚠️  Git hooks are not configured to use $(EXPECTED_HOOKS_PATH). Run 'make setup-hooks' to configure them."; \
	fi

# =============================================================================
# Code Generation
# =============================================================================

# Generate Go code from OpenAPI spec
gen-openapi: ## Generate OpenAPI code
	@command -v oapi-codegen >/dev/null 2>&1 || { echo "FAIL: oapi-codegen not installed - run: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest"; exit 1; }
	@OUTPUT=$$(oapi-codegen -config api/openapi/oapi-codegen.yaml api/openapi/mono.yaml 2>&1) || { echo "FAIL: OpenAPI code generation failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: OpenAPI code generated"

# Generate type-safe Go code from SQL queries using sqlc
gen-sqlc: ## Generate sqlc code
	@command -v sqlc >/dev/null 2>&1 || { echo "FAIL: sqlc not installed - run: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; exit 1; }
	@OUTPUT=$$(sqlc generate 2>&1) || { echo "FAIL: sqlc code generation failed"; echo "$$OUTPUT"; exit 1; }
	@for file in internal/infrastructure/persistence/postgres/sqlcgen/*.go; do \
		sed -i.bak -e '/"github.com\/jackc\/pgtype"/d' "$$file"; \
		rm "$$file.bak"; \
	done
	@for file in internal/infrastructure/persistence/postgres/sqlcgen/*.go; do \
		if grep -q 'sql.Null\[time.Time\]' "$$file"; then \
			if ! grep -q '"time"' "$$file"; then \
				sed -i.bak -e '/^import (/a\	"time"\n' "$$file"; \
				rm "$$file.bak"; \
			fi \
		fi \
	done
	@echo "OK: SQL type-safe code generated"

# Generate all code (OpenAPI, sqlc)
gen: gen-openapi gen-sqlc ## Generate all code (OpenAPI, sqlc)

# =============================================================================
# Code Quality
# =============================================================================

# Run go mod tidy to update dependencies
tidy: ## Run go mod tidy
	@OUTPUT=$$(go mod tidy 2>&1) || { echo "FAIL: go mod tidy failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Dependencies tidied"

# Format all Go files tracked in git
fmt: ## Format all Go files
	@FILES=$$(git ls-files '*.go' 2>/dev/null | while read f; do [ -f "$$f" ] && echo "$$f"; done); \
	if [ -n "$$FILES" ]; then \
		echo "$$FILES" | xargs gofmt -w; \
	fi
	@echo "OK: Go files formatted"

# Ensure all staged Go files are gofmt'ed (used by pre-commit hook)
fmt-check: ## Check formatting of staged Go files
	@STAGED_FILES=$$(git diff --cached --name-only --diff-filter=ACMR '*.go' 2>/dev/null || true); \
	if [ -z "$$STAGED_FILES" ]; then \
		echo "OK: No staged Go files to check"; exit 0; \
	fi; \
	UNFORMATTED=$$(echo "$$STAGED_FILES" | xargs gofmt -l 2>/dev/null || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "FAIL: Staged files not formatted"; \
		echo "$$UNFORMATTED"; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi; \
	echo "OK: Staged files formatted correctly"

# Check that ALL Go files are formatted (used by lint and check)
fmt-check-all:
	@FILES=$$(git ls-files '*.go' 2>/dev/null | while read f; do [ -f "$$f" ] && echo "$$f"; done); \
	if [ -z "$$FILES" ]; then \
		echo "OK: No Go files to check"; exit 0; \
	fi; \
	UNFORMATTED=$$(echo "$$FILES" | xargs gofmt -l 2>/dev/null || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "FAIL: Files not formatted"; \
		echo "$$UNFORMATTED"; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi; \
	echo "OK: All files formatted correctly"

# Run govulncheck to find vulnerabilities
security: ## Run security vulnerability check
	@go install golang.org/x/vuln/cmd/govulncheck@latest >/dev/null 2>&1
	@OUTPUT=$$(govulncheck ./... 2>&1) && echo "OK: No security vulnerabilities found" || { echo "FAIL: Security vulnerabilities detected"; echo "$$OUTPUT"; exit 1; }

# Build custom timezone linter
build-timeutc-linter:
	@OUTPUT=$$(cd tools/linters/timeutc && go build -o ../../../timeutc ./cmd/timeutc 2>&1) || { echo "FAIL: Failed to build timeutc linter"; echo "$$OUTPUT"; exit 1; }

# Build custom interface{} linter
build-nointerface-linter:
	@OUTPUT=$$(cd tools/linters/nointerface && go build -o ../../../nointerface ./cmd/nointerface 2>&1) || { echo "FAIL: Failed to build nointerface linter"; echo "$$OUTPUT"; exit 1; }

# Run interface{} linter (detects interface{} usage)
lint-interface: build-nointerface-linter ## Run interface{} linter
	@OUTPUT=$$(go list ./... | grep -v sqlcgen | xargs ./nointerface 2>&1) || { echo "FAIL: interface{} usage detected (use 'any' instead)"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: No interface{} usage found"

# Fix all interface{} by replacing with 'any'
lint-interface-fix: build-nointerface-linter ## Fix interface{} → any
	@go list ./... | grep -v sqlcgen | xargs ./nointerface -fix
	@echo "OK: All interface{} replaced with any"

# Run linters (fmt + golangci-lint + custom linters)
lint: build-timeutc-linter build-nointerface-linter ## Run all linters
	@$(MAKE) -s fmt-check-all >/dev/null 2>&1 || $(MAKE) -s fmt-check-all
	@golangci-lint config verify >/dev/null 2>&1
	@OUTPUT=$$(golangci-lint run 2>&1) || { echo "FAIL: golangci-lint found issues"; echo "$$OUTPUT"; exit 1; }
	@OUTPUT=$$(go list ./... | grep -v sqlcgen | xargs ./timeutc 2>&1) || { echo "FAIL: Non-UTC timezone usage detected"; echo "$$OUTPUT"; exit 1; }
	@OUTPUT=$$(go list ./... | grep -v sqlcgen | xargs ./nointerface 2>&1) || { echo "FAIL: interface{} usage detected (use 'any')"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: All linters passed"

# Configure git hooks to run automatically
setup-hooks: ## Configure git hooks
	git config core.hooksPath $(EXPECTED_HOOKS_PATH)
	@echo "Git hooks configured! Hooks in $(EXPECTED_HOOKS_PATH)/ will now run automatically."

# =============================================================================
# Testing
# =============================================================================

# Run unit tests only (no database required)
test: ## Run unit tests
	@OUTPUT=$$(go list ./... | grep -v '/tests/integration' | grep -v '/tests/e2e' | xargs go test 2>&1) || { echo "FAIL: Unit tests failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Unit tests passed"

# Run unit tests with race detector
test-race: ## Run unit tests with race detector
	@OUTPUT=$$(go list ./... | grep -v '/tests/integration' | grep -v '/tests/e2e' | xargs go test -race 2>&1) || { echo "FAIL: Unit tests with race detector failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Unit tests passed (with race detector)"

# Run a single test by name
# Usage: make test-one RUN=TestName
#        make test-one RUN=TestName PKG=./path/to/package
test-one: ## Run a single test (RUN=TestName PKG=./path)
ifndef RUN
	$(error RUN is required. Usage: make test-one RUN=TestName)
endif
	@if ! docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then \
		$(MAKE) -s test-integration-up >/dev/null 2>&1 || { echo "FAIL: Test database not running - run 'make test-integration-up' first"; exit 1; }; \
	fi
	@MONO_STORAGE_DSN="$(TEST_DSN)" go test -run "$(RUN)" $(PKG) -count=1 -v
	@echo "OK: Test $(RUN) passed"

# Run benchmarks (requires MONO_STORAGE_DSN env var)
bench: ## Run benchmarks
	@echo "Running benchmarks..."
	@if [ -z "$${MONO_STORAGE_DSN:-}" ]; then \
		echo "Warning: MONO_STORAGE_DSN not set. Set it to run benchmarks with real database."; \
		echo "Usage: MONO_STORAGE_DSN='postgres://user:pass@localhost:5432/dbname' make bench"; \
		echo "Skipping benchmarks..."; \
	else \
		MONO_STORAGE_DSN=$${MONO_STORAGE_DSN} go test -bench=. -benchmem ./tests/integration/postgres; \
	fi

# Run benchmarks using test database (port 5433, auto-cleanup)
bench-test: ## Run benchmarks using test database
	@trap '$(MAKE) test-integration-clean >/dev/null 2>&1' EXIT; \
	docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true; \
	$(MAKE) test-integration-up >/dev/null 2>&1; \
	MONO_STORAGE_DSN="$(TEST_DSN)" go test -bench=. -benchmem ./tests/integration/postgres; \
	echo "OK: Benchmarks"

# =============================================================================
# Profile-Guided Optimization (PGO)
# =============================================================================

# Collect CPU profile for PGO optimization (uses test database)
pgo-collect: ## Collect CPU profile for PGO
	@echo "=== Collecting CPU profile for PGO ==="
	@echo ""
	@echo "=== Cleaning any existing test database ==="
	@docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	@echo ""
	@echo "=== Starting fresh test database ==="
	@$(MAKE) test-integration-up
	@echo ""
	@echo "=== Running benchmarks to collect CPU profile (30s per package) ==="
	@rm -f pgo-*.prof
	@echo "Profiling integration/postgres benchmarks..."
	@MONO_STORAGE_DSN="$(TEST_DSN)" go test -cpuprofile=pgo-integration.prof -bench=. -benchtime=30s ./tests/integration/postgres || true
	@echo "Profiling internal/application/auth benchmarks..."
	@go test -cpuprofile=pgo-auth.prof -bench=. -benchtime=10s ./internal/application/auth || true
	@echo "Profiling internal/infrastructure/http/response benchmarks..."
	@go test -cpuprofile=pgo-response.prof -bench=. -benchtime=10s ./internal/infrastructure/http/response || true
	@echo ""
	@echo "=== Merging profiles ==="
	@go tool pprof -proto pgo-*.prof > merged.pgo 2>/dev/null || cp pgo-integration.prof merged.pgo
	@rm -f pgo-*.prof
	@echo ""
	@echo "=== Cleaning up test database ==="
	@$(MAKE) test-integration-clean
	@echo ""
	@echo "=== Installing PGO profiles ==="
	@mv merged.pgo cmd/server/default.pgo
	@cp cmd/server/default.pgo cmd/worker/default.pgo
	@echo "✅ PGO profiles saved to:"
	@echo "   - cmd/server/default.pgo"
	@echo "   - cmd/worker/default.pgo"
	@echo ""
	@echo "Run 'make build' to compile with PGO optimization."

# Collect PGO profile and build optimized binaries
pgo-build: pgo-collect build build-worker ## Collect PGO and build optimized binaries
	@echo ""
	@echo "✅ Built with PGO optimization:"
	@echo "   - $(BINARY_NAME)"
	@echo "   - $(WORKER_BINARY_NAME)"

# Remove PGO profile files
pgo-clean: ## Remove PGO profiles
	@echo "Removing PGO profiles..."
	@rm -f cmd/server/default.pgo cmd/worker/default.pgo
	@rm -f pgo-*.prof merged.pgo cpu.pgo
	@echo "✅ PGO profiles removed"

# =============================================================================
# Building
# =============================================================================

# Build the server binary
build: gen ## Build the server binary
	@OUTPUT=$$(go build -o $(BINARY_NAME) ./cmd/server 2>&1) || { echo "FAIL: Server binary compilation failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Server binary built ($(BINARY_NAME))"

# Build the background worker binary
build-worker: ## Build the worker binary
	@OUTPUT=$$(go build -o $(WORKER_BINARY_NAME) cmd/worker/main.go 2>&1) || { echo "FAIL: Worker binary compilation failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Worker binary built ($(WORKER_BINARY_NAME))"

# Build the API key generator tool
build-apikey: ## Build API key generator
	@OUTPUT=$$(go build -o mono-apikey cmd/apikey/main.go 2>&1) || { echo "FAIL: API key tool compilation failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: API key generator built"

# Generate a new API key
# Usage: make gen-apikey NAME=keyname
#        make gen-apikey NAME=keyname DAYS=30
gen-apikey: build-apikey ## Generate API key (NAME=name DAYS=n)
ifndef NAME
	$(error NAME is required. Usage: make gen-apikey NAME=keyname)
endif
	@if [ -z "$(DAYS)" ]; then \
		MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" ./mono-apikey -name "$(NAME)"; \
	else \
		MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" ./mono-apikey -name "$(NAME)" -days $(DAYS); \
	fi

# [PROD] Generate API key using Docker and .env file
docker-gen-apikey: ## [PROD] Generate API key via Docker (NAME=name DAYS=n)
ifndef NAME
	$(error NAME is required. Usage: make docker-gen-apikey NAME=keyname)
endif
	@if [ -z "$(DAYS)" ]; then \
		docker compose -f docker-compose.prod.yml run --rm -e NAME="$(NAME)" apikey -name "$(NAME)"; \
	else \
		docker compose -f docker-compose.prod.yml run --rm -e NAME="$(NAME)" -e DAYS=$(DAYS) apikey -name "$(NAME)" -days $(DAYS); \
	fi

# Build and run server using dev database
run: build ## Build and run server
	@echo "Running server..."
	MONO_STORAGE_DSN="$(DEV_STORAGE_DSN)" MONO_HTTP_PORT=8081 MONO_OTEL_ENABLED=false ./$(BINARY_NAME)

# Remove built binaries and PGO profiles
clean: ## Remove built binaries and PGO profiles
	@echo "Cleaning up..."
	rm -f mono-server mono-worker mono-apikey timeutc nointerface
	rm -f cmd/server/default.pgo cmd/worker/default.pgo
	rm -f pgo-*.prof merged.pgo cpu.pgo
	@echo "All binaries and PGO profiles removed"

# =============================================================================
# Docker Development
# =============================================================================

# Build the Docker image
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .

# Run the Docker container
docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 8081:8081 $(DOCKER_IMAGE)

# =============================================================================
# Production Docker Compose Commands
# =============================================================================

# [PROD] Start production services (server, worker, postgres)
docker-up: ## [PROD] Start production services
	@echo "Starting production services..."
	docker compose -f docker-compose.prod.yml up -d
	@echo "✅ Services started. Use 'make docker-logs' to view logs"

# [PROD] Rebuild images and start services (use after git pull)
docker-build-up: ## [PROD] Rebuild and start services
	@echo "Rebuilding images and starting services..."
	docker compose -f docker-compose.prod.yml --profile tools up -d --build
	@echo "✅ Services rebuilt and started. Use 'make docker-logs' to view logs"

# [PROD] Rebuild images only (without starting)
docker-rebuild: ## [PROD] Rebuild Docker images
	@echo "Rebuilding Docker images..."
	docker compose -f docker-compose.prod.yml --profile tools build

# [PROD] Stop production services
docker-down: ## [PROD] Stop production services
	@echo "Stopping production services..."
	docker compose -f docker-compose.prod.yml down

# [PROD] Restart production services
# Usage: make docker-restart           (restarts all)
#        make docker-restart SERVICE=server
docker-restart: ## [PROD] Restart services (SERVICE=name)
	@if [ -z "$(SERVICE)" ]; then \
		echo "Restarting all services..."; \
		docker compose -f docker-compose.prod.yml restart; \
	else \
		echo "Restarting $(SERVICE)..."; \
		docker compose -f docker-compose.prod.yml restart $(SERVICE); \
	fi

# [PROD] Restart server only (fast, skips migration)
docker-restart-server: ## [PROD] Restart server only
	@echo "Restarting server..."
	docker compose -f docker-compose.prod.yml up -d server --force-recreate

# [PROD] Restart workers only (fast, skips migration)
docker-restart-workers: ## [PROD] Restart workers only
	@echo "Restarting workers..."
	docker compose -f docker-compose.prod.yml up -d $(WORKERS) --force-recreate

# [PROD] View logs
# Usage: make docker-logs           (all services)
#        make docker-logs SERVICE=server
docker-logs: ## [PROD] View logs (SERVICE=name)
	@if [ -z "$(SERVICE)" ]; then \
		echo "Showing logs for all services (Ctrl+C to exit)..."; \
		docker compose -f docker-compose.prod.yml logs -f; \
	else \
		echo "Showing logs for $(SERVICE) (Ctrl+C to exit)..."; \
		docker compose -f docker-compose.prod.yml logs -f $(SERVICE); \
	fi

# [PROD] View server logs only
docker-logs-server: ## [PROD] View server logs
	@echo "Showing logs for server (Ctrl+C to exit)..."
	docker compose -f docker-compose.prod.yml logs -f server

# [PROD] View worker logs only (all workers)
docker-logs-workers: ## [PROD] View worker logs
	@echo "Showing logs for workers (Ctrl+C to exit)..."
	docker compose -f docker-compose.prod.yml logs -f $(WORKERS)

# [PROD] View postgres logs only
docker-logs-postgres: ## [PROD] View postgres logs
	@echo "Showing logs for postgres (Ctrl+C to exit)..."
	docker compose -f docker-compose.prod.yml logs -f postgres

# [PROD] Show running containers
docker-ps: ## [PROD] Show running containers
	docker compose -f docker-compose.prod.yml ps

# [PROD] Stop and remove all containers and volumes (WARNING: deletes data!)
docker-clean: ## [PROD] Remove containers and volumes (DESTRUCTIVE)
	@echo "⚠️  WARNING: This will delete all data in the database!"
	@echo "Press Ctrl+C to cancel, or Enter to continue..."
	@read dummy; \
	echo "Cleaning up production deployment..."; \
	docker compose -f docker-compose.prod.yml --profile tools down -v; \
	echo "✅ All containers and volumes removed"

# [PROD] Open shell in server container
docker-shell-server: ## [PROD] Shell into server container
	docker compose -f docker-compose.prod.yml exec server sh

# [PROD] Open shell in worker container (defaults to worker-1)
# Usage: make docker-shell-worker WORKER=worker-2
docker-shell-worker: ## [PROD] Shell into worker (WORKER=name)
	docker compose -f docker-compose.prod.yml exec $(WORKER) sh

# [PROD] Open PostgreSQL shell
docker-shell-postgres: ## [PROD] Shell into postgres
	@USER=$${POSTGRES_USER:-mono}; \
	DB=$${POSTGRES_DB:-mono_db}; \
	docker compose -f docker-compose.prod.yml exec postgres psql -U $$USER -d $$DB

# [PROD] Check health status of all services
docker-health: ## [PROD] Check service health
	@echo "Checking health status of production services..."
	docker compose -f docker-compose.prod.yml ps

# [PROD] Test server /health endpoint (detects HTTP/HTTPS from SERVER_PORT)
docker-health-server: ## [PROD] Test server health endpoint
	@echo "Testing server health endpoint..."; \
	if [ -f .env ]; then \
		. ./.env 2>/dev/null || true; \
	fi; \
	PORT=$${SERVER_PORT:-80}; \
	if [ "$${MONO_TLS_ENABLED}" = "true" ]; then \
		echo "Testing HTTPS endpoint at https://localhost:$$PORT/health"; \
		curl -f -k -s https://localhost:$$PORT/health && echo -e "\n✅ Server is healthy" || echo -e "\n❌ Health check failed"; \
	else \
		echo "Testing HTTP endpoint at http://localhost:$$PORT/health"; \
		curl -f -s http://localhost:$$PORT/health && echo -e "\n✅ Server is healthy" || echo -e "\n❌ Health check failed"; \
	fi

# =============================================================================
# Migration Image (goose-migrate) - Multi-architecture
# =============================================================================

# Setup Docker buildx for multi-platform builds
docker-buildx-setup: ## Setup Docker buildx for multi-arch
	@echo "Setting up Docker buildx builder..."; \
	if ! docker buildx inspect multiarch-builder > /dev/null 2>&1; then \
		docker buildx create --name multiarch-builder --driver docker-container --bootstrap --use; \
		echo "✅ Created and activated multiarch-builder"; \
	else \
		docker buildx use multiarch-builder; \
		echo "✅ Activated existing multiarch-builder"; \
	fi; \
	docker buildx inspect --bootstrap

# Build goose migration image for current platform only (for local testing)
docker-build-migrate: ## Build migration image for current platform
	@echo "Building migration image $(MIGRATE_IMAGE):$(MIGRATE_IMAGE_TAG) for current platform..."; \
	GIT_COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	docker buildx build \
		-f Dockerfile.migrate \
		--build-arg GOOSE_VERSION=$(GOOSE_VERSION) \
		--build-arg BUILD_DATE=$$BUILD_DATE \
		--build-arg GIT_COMMIT=$$GIT_COMMIT \
		-t $(MIGRATE_IMAGE):$(MIGRATE_IMAGE_TAG) \
		-t $(MIGRATE_IMAGE):latest \
		--load \
		.; \
	echo "✅ Built $(MIGRATE_IMAGE):$(MIGRATE_IMAGE_TAG) for current platform"

# Build and push goose migration image to ghcr.io (multi-arch: amd64, arm64)
docker-push-migrate: docker-buildx-setup ## Build and push multi-arch migration image
	@echo "Building and pushing multi-arch migration image to GitHub Container Registry..."; \
	echo "Note: Make sure you're logged in with: docker login ghcr.io -u USERNAME"; \
	GIT_COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	BUILD_DATE=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	docker buildx build \
		--builder multiarch-builder \
		--platform linux/amd64,linux/arm64 \
		-f Dockerfile.migrate \
		--build-arg GOOSE_VERSION=$(GOOSE_VERSION) \
		--build-arg BUILD_DATE=$$BUILD_DATE \
		--build-arg GIT_COMMIT=$$GIT_COMMIT \
		-t $(MIGRATE_IMAGE):$(MIGRATE_IMAGE_TAG) \
		-t $(MIGRATE_IMAGE):latest \
		--push \
		.; \
	echo "✅ Pushed $(MIGRATE_IMAGE):$(MIGRATE_IMAGE_TAG) and $(MIGRATE_IMAGE):latest (amd64, arm64)"

# =============================================================================
# Development Database (port 5432)
# =============================================================================

# [DEV DB] Start development database (port 5432)
db-up: ## [DEV] Start development database
	@echo "Starting PostgreSQL database..."
	docker compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3
	@echo "✅ Database ready at $(DEV_STORAGE_DSN)"

# [DEV DB] Stop development database
db-down: ## [DEV] Stop development database
	@echo "Stopping database containers..."
	docker compose down

# [DEV DB] Stop and remove development database with all data
db-clean: ## [DEV] Remove database with all data
	@echo "Cleaning up development database and volumes..."
	docker compose down -v
	@echo "Development database cleaned!"

# =============================================================================
# Database Migrations
# =============================================================================

# Run migrations
# Usage: make db-migrate-up DB_URL="postgres://..."
db-migrate-up: ## Run migrations (DB_URL=...)
ifndef DB_URL
	$(error DB_URL is required. Usage: make db-migrate-up DB_URL="postgres://...")
endif
	@echo "Running migrations up..."
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations $(DB_DRIVER) "$(DB_URL)" up

# Rollback migration
# Usage: make db-migrate-down DB_URL="postgres://..."
db-migrate-down: ## Rollback migration (DB_URL=...)
ifndef DB_URL
	$(error DB_URL is required. Usage: make db-migrate-down DB_URL="postgres://...")
endif
	@echo "Rolling back migration..."
	go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations $(DB_DRIVER) "$(DB_URL)" down

# Create migration
# Usage: make db-migrate-create NAME=add_column_xyz
db-migrate-create: ## Create migration (NAME=...)
ifndef NAME
	$(error NAME is required. Usage: make db-migrate-create NAME=migration_name)
endif
	@echo "Creating new migration: $(NAME)"
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations create $(NAME) sql

# =============================================================================
# Test Database (port 5433)
# =============================================================================

# [TEST DB] Start test database (port 5433)
test-integration-up: ## [TEST] Start test database
	@echo "Starting PostgreSQL test database..."
	@docker compose -f docker-compose.test.yml up -d
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
	@MONO_STORAGE_DSN="$(TEST_DSN)" \
		go run github.com/pressly/goose/v3/cmd/goose@latest \
		-dir internal/infrastructure/persistence/postgres/migrations \
		postgres \
		"$(TEST_DSN)" \
		up

# [TEST DB] Stop test database
test-integration-down: ## [TEST] Stop test database
	@echo "Stopping PostgreSQL test database..."
	docker compose -f docker-compose.test.yml down

# [TEST DB] Stop test database (data is ephemeral via tmpfs)
test-integration-clean: ## [TEST] Clean test database
	@echo "Stopping PostgreSQL test database..."
	docker compose -f docker-compose.test.yml down
	@echo "Test database stopped (data was in tmpfs, already gone)"

# Run integration tests (requires MONO_STORAGE_DSN env var)
test-integration-run:
	@if [ -z "$${MONO_STORAGE_DSN:-}" ]; then \
		echo "Error: MONO_STORAGE_DSN is required."; \
		exit 1; \
	fi
	go test -p 1 ./tests/integration/... -count=1
	@echo "OK: Integration tests"

# [TEST DB] Run integration tests (auto-cleanup before/after)
test-integration: ## Run integration tests with test database
	@trap '$(MAKE) -s test-integration-clean >/dev/null 2>&1 || true' EXIT; \
	docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true; \
	OUTPUT=$$($(MAKE) -s test-integration-up 2>&1) || { echo "FAIL: Could not start test database"; echo "$$OUTPUT"; exit 1; }; \
	OUTPUT=$$(MONO_STORAGE_DSN="$(TEST_DSN)" go test -p 1 ./tests/integration/... -count=1 2>&1) || { echo "FAIL: Integration tests failed"; echo "$$OUTPUT"; exit 1; }; \
	echo "OK: Integration tests passed"

# [TEST DB] Run HTTP integration tests (auto-cleanup before/after)
test-integration-http: ## Run HTTP integration tests
	@trap '$(MAKE) -s test-integration-clean >/dev/null 2>&1 || true' EXIT; \
	docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true; \
	OUTPUT=$$($(MAKE) -s test-integration-up 2>&1) || { echo "FAIL: Could not start test database"; echo "$$OUTPUT"; exit 1; }; \
	OUTPUT=$$(MONO_STORAGE_DSN="$(TEST_DSN)" go test ./tests/integration/http -count=1 2>&1) || { echo "FAIL: HTTP integration tests failed"; echo "$$OUTPUT"; exit 1; }; \
	echo "OK: HTTP integration tests passed"

# [TEST DB] Run end-to-end tests (auto-cleanup before/after)
test-e2e: ## Run end-to-end tests
	@trap '$(MAKE) -s test-integration-clean >/dev/null 2>&1 || true' EXIT; \
	docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true; \
	OUTPUT=$$($(MAKE) -s test-integration-up 2>&1) || { echo "FAIL: Could not start test database"; echo "$$OUTPUT"; exit 1; }; \
	OUTPUT=$$(MONO_STORAGE_DSN="$(TEST_DSN)" go test ./tests/e2e -count=1 2>&1) || { echo "FAIL: End-to-end tests failed"; echo "$$OUTPUT"; exit 1; }; \
	echo "OK: End-to-end tests passed"

# Run SQL storage tests (requires running database)
test-sql: ## Run SQL storage tests
	@OUTPUT=$$(go test ./internal/infrastructure/persistence/postgres/... 2>&1) || { echo "FAIL: SQL storage tests failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: SQL storage tests passed"

# Run all tests (unit, race, integration, e2e) - excludes benchmarks
test-all: test test-race test-integration test-integration-http test-e2e ## Run all tests
	@echo "OK: All tests passed"

# Run all tests including benchmarks (slow, use sparingly)
test-all-bench: test-all bench-test ## Run all tests including benchmarks
	@echo "OK: All tests and benchmarks passed"

# =============================================================================
# Test Database Helpers
# =============================================================================

# [TEST DB] Show test database container status
test-db-status: ## [TEST] Show test database status
	docker compose -f docker-compose.test.yml ps

# Show PostgreSQL test database logs
test-db-logs: ## [TEST] Show test database logs
	docker compose -f docker-compose.test.yml logs -f postgres

# Connect to PostgreSQL test database shell
test-db-shell: ## [TEST] Connect to test database shell
	docker compose -f docker-compose.test.yml exec postgres psql -U postgres -d mono_test

# =============================================================================
# Documentation
# =============================================================================

# Sync CLAUDE.md to AGENTS.md
sync-agents: ## Sync CLAUDE.md to AGENTS.md
	@echo "Syncing agent instruction files..."
	cp CLAUDE.md AGENTS.md
	@echo "Synced CLAUDE.md to AGENTS.md"

# =============================================================================
# Docker Sandbox (AI Agent Development)
# =============================================================================
# Docker Sandboxes provide isolated environments for running AI coding agents
# like Claude Code safely on your local machine.
#
# Requirements:
#   - Docker Desktop 4.50 or later
#   - Enable experimental features in Docker Desktop settings
# =============================================================================

# Build the custom sandbox template with project dependencies
sandbox-build: ## Build custom sandbox template for Claude Code
	@echo "Building custom sandbox template..."
	@OUTPUT=$$(docker build -f Dockerfile.sandbox -t mono-sandbox . 2>&1) || { echo "FAIL: Sandbox template build failed"; echo "$$OUTPUT"; exit 1; }
	@echo "OK: Sandbox template built (mono-sandbox)"

# Run Claude Code in the custom sandbox with Docker socket access
sandbox-run: ## Run Claude Code in sandbox (with Docker access)
	@echo "Starting Claude Code in sandbox..."
	@echo "Tip: Use 'docker sandbox ls' to see running sandboxes"
	docker sandbox run --template mono-sandbox --mount-docker-socket claude

# Run Claude Code in sandbox without Docker socket (more secure)
sandbox-run-secure: ## Run Claude Code in sandbox (no Docker access)
	@echo "Starting Claude Code in sandbox (secure mode - no Docker access)..."
	docker sandbox run --template mono-sandbox claude

# Run Claude Code with a specific prompt
# Usage: make sandbox-prompt PROMPT="Add error handling to the login function"
sandbox-prompt: ## Run Claude Code with prompt (PROMPT="...")
ifndef PROMPT
	$(error PROMPT is required. Usage: make sandbox-prompt PROMPT="your prompt here")
endif
	docker sandbox run --template mono-sandbox --mount-docker-socket claude "$(PROMPT)"

# Continue previous Claude Code session
sandbox-continue: ## Continue previous Claude Code session
	@echo "Continuing previous Claude Code session..."
	docker sandbox run --template mono-sandbox --mount-docker-socket claude -c

# List all sandboxes
sandbox-ls: ## List all Docker sandboxes
	docker sandbox ls

# Show sandbox details
# Usage: make sandbox-inspect ID=<sandbox-id>
sandbox-inspect: ## Inspect sandbox details (ID=<sandbox-id>)
ifndef ID
	$(error ID is required. Usage: make sandbox-inspect ID=<sandbox-id>)
endif
	docker sandbox inspect $(ID)

# Remove a specific sandbox
# Usage: make sandbox-rm ID=<sandbox-id>
sandbox-rm: ## Remove a sandbox (ID=<sandbox-id>)
ifndef ID
	$(error ID is required. Usage: make sandbox-rm ID=<sandbox-id>)
endif
	@echo "Removing sandbox $(ID)..."
	docker sandbox rm $(ID)
	@echo "OK: Sandbox removed"

# Remove all sandboxes (use with caution)
sandbox-clean: ## Remove all sandboxes (DESTRUCTIVE)
	@echo "⚠️  WARNING: This will remove ALL Docker sandboxes!"
	@echo "Press Ctrl+C to cancel, or Enter to continue..."
	@read dummy; \
	SANDBOXES=$$(docker sandbox ls -q 2>/dev/null); \
	if [ -n "$$SANDBOXES" ]; then \
		docker sandbox rm $$SANDBOXES; \
		echo "OK: All sandboxes removed"; \
	else \
		echo "OK: No sandboxes to remove"; \
	fi

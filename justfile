# Binary names
binary_name := "mono-server"
worker_binary_name := "mono-worker"
docker_image := "mono-service"

# Production worker instances (add worker-3, worker-4, etc. here)
workers := "worker-1 worker-2"

# Migration image configuration
goose_version := "v3.26.0"
migrate_image := "ghcr.io/rezkam/goose-migrate"
migrate_image_tag := goose_version

# Default DB Driver
db_driver := env_var_or_default("DB_DRIVER", "postgres")

# Development database DSN
dev_storage_dsn := env_var_or_default("DEV_STORAGE_DSN", "postgres://mono:mono_password@localhost:5432/mono_db?sslmode=disable")

# Test database DSN
test_dsn := "postgres://postgres:postgres@localhost:5433/mono_test?sslmode=disable"

# Git hooks configuration
expected_hooks_path := ".githooks"

# Color output
export FORCE_COLOR := "1"

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
#    - Commands: just db-up, just db-down
#    - Purpose: Local development, manual testing, persistent data
#
# 2. TEST DATABASE (docker-compose.test.yml)
#    - Port: 5433
#    - Container: mono-postgres-test
#    - Database: mono_test
#    - User: postgres
#    - Commands: just test-integration, just test-integration-up/down/clean
#    - Purpose: Local automated tests, wiped between test runs
#
# Both databases can run simultaneously on different ports.
# =============================================================================

# Default recipe (shows help)
default:
    @just --list

# Run everything: build, lint, and all tests (minimal output, fail-fast)
[no-exit-message]
check:
    #!/usr/bin/env bash
    set -euo pipefail
    printf "Building... "; OUTPUT=$(just -q build 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    printf "Linting... "; OUTPUT=$(just -q lint 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    printf "Unit tests... "; OUTPUT=$(go list ./... | grep -v '/tests/' | xargs go test 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    printf "Integration (postgres)... "; OUTPUT=$(just -q test-integration 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    printf "Integration (http)... "; OUTPUT=$(just -q test-integration-http 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    if docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres >/dev/null 2>&1; then
        printf "Security tests... "; OUTPUT=$(MONO_STORAGE_DSN="{{test_dsn}}" go test ./tests/security -count=1 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    fi
    printf "E2E tests... "; OUTPUT=$(just -q test-e2e 2>&1) && echo "OK" || { echo "FAIL"; echo "$OUTPUT"; exit 1; }
    echo ""
    echo "OK: All checks passed"

# Display this help message
help:
    @echo "Usage: just <recipe>"
    @echo ""
    @echo "Available recipes:"
    @just --list

# Check if git hooks are properly configured
[private]
check-hooks:
    #!/usr/bin/env bash
    set -euo pipefail
    HOOKS_PATH=$(git config core.hooksPath || echo "")
    if [ "$HOOKS_PATH" != "{{expected_hooks_path}}" ]; then
        echo "⚠️  Git hooks are not configured to use {{expected_hooks_path}}. Run 'just setup-hooks' to configure them."
    fi

# =============================================================================
# Code Generation
# =============================================================================

# Generate Go code from OpenAPI spec
gen-openapi:
    @echo "Generating OpenAPI code..."
    @command -v oapi-codegen >/dev/null 2>&1 || { echo "Error: oapi-codegen is not installed. Install with: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest"; exit 1; }
    oapi-codegen -config api/openapi/oapi-codegen.yaml api/openapi/mono.yaml

# Generate type-safe Go code from SQL queries using sqlc
gen-sqlc:
    @echo "Generating sqlc code..."
    @command -v sqlc >/dev/null 2>&1 || { echo "Error: sqlc is not installed. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; exit 1; }
    sqlc generate
    @echo "Fixing sqlc imports (removing legacy pgtype)..."
    @for file in internal/infrastructure/persistence/postgres/sqlcgen/*.go; do \
        sed -i.bak -e '/"github.com\/jackc\/pgtype"/d' "$file"; \
        rm "$file.bak"; \
    done
    @echo "Adding time import for sql.Null[time.Time] support..."
    @for file in internal/infrastructure/persistence/postgres/sqlcgen/*.go; do \
        if grep -q 'sql.Null\[time.Time\]' "$file"; then \
            if ! grep -q '"time"' "$file"; then \
                sed -i.bak -e '/^import (/a\	"time"\n' "$file"; \
                rm "$file.bak"; \
            fi \
        fi \
    done

# Generate all code (OpenAPI, sqlc)
gen: gen-openapi gen-sqlc

# =============================================================================
# Code Quality
# =============================================================================

# Run go mod tidy to update dependencies
tidy:
    @echo "Tidying module dependencies..."
    go mod tidy

# Format all Go files tracked in git
fmt:
    @echo "Formatting Go files..."
    @FILES=$(git ls-files '*.go' 2>/dev/null | while read f; do [ -f "$f" ] && echo "$f"; done); \
    if [ -n "$FILES" ]; then \
        echo "$FILES" | xargs gofmt -w; \
    else \
        echo "No Go files to format"; \
    fi

# Ensure all staged Go files are gofmt'ed (used by pre-commit hook)
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    STAGED_FILES=$(git diff --cached --name-only --diff-filter=ACMR '*.go' 2>/dev/null || true)
    if [ -z "$STAGED_FILES" ]; then
        exit 0
    fi
    UNFORMATTED=$(echo "$STAGED_FILES" | xargs gofmt -l 2>/dev/null || true)
    if [ -n "$UNFORMATTED" ]; then
        echo "FAIL: Formatting errors"
        echo "$UNFORMATTED"
        echo "Run 'just fmt' to fix."
        exit 1
    fi
    echo "OK: All staged files formatted"

# Check that ALL Go files are formatted (used by lint and check)
[no-exit-message]
fmt-check-all:
    #!/usr/bin/env bash
    set -euo pipefail
    FILES=$(git ls-files '*.go' 2>/dev/null | while read f; do [ -f "$f" ] && echo "$f"; done)
    if [ -z "$FILES" ]; then
        exit 0
    fi
    UNFORMATTED=$(echo "$FILES" | xargs gofmt -l 2>/dev/null || true)
    if [ -n "$UNFORMATTED" ]; then
        echo "FAIL: Unformatted files:"
        echo "$UNFORMATTED"
        echo "Run 'just fmt' to fix."
        exit 1
    fi
    echo "OK: All files formatted"

# Run govulncheck to find vulnerabilities
security:
    #!/usr/bin/env bash
    set -euo pipefail
    go install golang.org/x/vuln/cmd/govulncheck@latest >/dev/null 2>&1
    OUTPUT=$(govulncheck ./... 2>&1) && echo "OK: No vulnerabilities" || { echo "FAIL: Vulnerabilities found"; echo "$OUTPUT"; exit 1; }

# Build custom timezone linter
build-timeutc-linter:
    @echo "Building timeutc linter..."
    cd tools/linters/timeutc && go build -o ../../../timeutc ./cmd/timeutc

# Build custom interface{} linter
build-nointerface-linter:
    @echo "Building nointerface linter..."
    cd tools/linters/nointerface && go build -o ../../../nointerface ./cmd/nointerface

# Run interface{} linter (detects interface{} usage)
lint-interface: build-nointerface-linter
    @echo "Running custom interface{} linter..."
    go list ./... | grep -v sqlcgen | xargs ./nointerface

# Fix all interface{} by replacing with 'any'
lint-interface-fix: build-nointerface-linter
    @echo "Automatically fixing interface{} → any..."
    go list ./... | grep -v sqlcgen | xargs ./nointerface -fix
    @echo "✅ All interface{} replaced with 'any'"

# Run linters (fmt + golangci-lint + custom linters)
[no-exit-message]
lint: build-timeutc-linter build-nointerface-linter
    #!/usr/bin/env bash
    set -euo pipefail
    just fmt-check-all
    golangci-lint config verify >/dev/null 2>&1
    OUTPUT=$(golangci-lint run 2>&1) || { echo "FAIL: golangci-lint"; echo "$OUTPUT"; exit 1; }
    OUTPUT=$(go list ./... | grep -v sqlcgen | xargs ./timeutc 2>&1) || { echo "FAIL: Timezone linter"; echo "$OUTPUT"; exit 1; }
    OUTPUT=$(go list ./... | grep -v sqlcgen | xargs ./nointerface 2>&1) || { echo "FAIL: Interface linter"; echo "$OUTPUT"; exit 1; }
    echo "OK: All linters passed"

# Configure git hooks to run automatically
setup-hooks:
    git config core.hooksPath {{expected_hooks_path}}
    @echo "Git hooks configured! Hooks in {{expected_hooks_path}}/ will now run automatically."

# =============================================================================
# Testing
# =============================================================================

# Run unit tests only (no database required)
[no-exit-message]
test:
    #!/usr/bin/env bash
    set -euo pipefail
    go list ./... | grep -v '/tests/integration' | grep -v '/tests/e2e' | xargs go test
    echo "OK: Unit tests"

# Run unit tests with race detector
[no-exit-message]
test-race:
    #!/usr/bin/env bash
    set -euo pipefail
    go list ./... | grep -v '/tests/integration' | grep -v '/tests/e2e' | xargs go test -race
    echo "OK: Unit tests (race)"

# Run a specific test
[group('test')]
[no-exit-message]
test-one RUN PKG="./tests/integration/...":
    #!/usr/bin/env bash
    set -euo pipefail
    # Ensure test database is running
    if ! docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then
        echo "Test database not running. Starting it..."
        just test-integration-up
    fi
    echo "Running test '{{RUN}}' in {{PKG}}..."
    MONO_STORAGE_DSN="{{test_dsn}}" go test -run "{{RUN}}" {{PKG}} -count=1 -v
    echo "OK: {{RUN}}"

# Run benchmarks (requires MONO_STORAGE_DSN env var)
[no-exit-message]
bench:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Running benchmarks..."
    if [ -z "${MONO_STORAGE_DSN:-}" ]; then
        echo "Warning: MONO_STORAGE_DSN not set. Set it to run benchmarks with real database."
        echo "Usage: MONO_STORAGE_DSN='postgres://user:pass@localhost:5432/dbname' just bench"
        echo "Skipping benchmarks..."
    else
        MONO_STORAGE_DSN=${MONO_STORAGE_DSN} go test -bench=. -benchmem ./tests/integration/postgres
    fi

# Run benchmarks using test database (port 5433, auto-cleanup)
[group('test')]
[no-exit-message]
bench-test:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'just test-integration-clean >/dev/null 2>&1' EXIT
    docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
    just test-integration-up >/dev/null 2>&1
    MONO_STORAGE_DSN="{{test_dsn}}" go test -bench=. -benchmem ./tests/integration/postgres
    echo "OK: Benchmarks"

# =============================================================================
# Profile-Guided Optimization (PGO)
# =============================================================================

# Collect CPU profile for PGO optimization (uses test database)
[group('pgo')]
pgo-collect:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "=== Collecting CPU profile for PGO ==="
    echo ""
    echo "=== Cleaning any existing test database ==="
    docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
    echo ""
    echo "=== Starting fresh test database ==="
    just test-integration-up
    echo ""
    echo "=== Running benchmarks to collect CPU profile (30s per package) ==="
    rm -f pgo-*.prof
    echo "Profiling integration/postgres benchmarks..."
    MONO_STORAGE_DSN="{{test_dsn}}" \
        go test -cpuprofile=pgo-integration.prof -bench=. -benchtime=30s ./tests/integration/postgres || true
    echo "Profiling internal/application/auth benchmarks..."
    go test -cpuprofile=pgo-auth.prof -bench=. -benchtime=10s ./internal/application/auth || true
    echo "Profiling internal/infrastructure/http/response benchmarks..."
    go test -cpuprofile=pgo-response.prof -bench=. -benchtime=10s ./internal/infrastructure/http/response || true
    echo ""
    echo "=== Merging profiles ==="
    go tool pprof -proto pgo-*.prof > merged.pgo 2>/dev/null || cp pgo-integration.prof merged.pgo
    rm -f pgo-*.prof
    echo ""
    echo "=== Cleaning up test database ==="
    just test-integration-clean
    echo ""
    echo "=== Installing PGO profiles ==="
    mv merged.pgo cmd/server/default.pgo
    cp cmd/server/default.pgo cmd/worker/default.pgo
    echo "✅ PGO profiles saved to:"
    echo "   - cmd/server/default.pgo"
    echo "   - cmd/worker/default.pgo"
    echo ""
    echo "Run 'just build' to compile with PGO optimization."

# Collect PGO profile and build optimized binaries
[group('pgo')]
pgo-build: pgo-collect build build-worker
    @echo ""
    @echo "✅ Built with PGO optimization:"
    @echo "   - {{binary_name}}"
    @echo "   - {{worker_binary_name}}"

# Remove PGO profile files
[group('pgo')]
pgo-clean:
    @echo "Removing PGO profiles..."
    rm -f cmd/server/default.pgo cmd/worker/default.pgo
    rm -f pgo-*.prof merged.pgo cpu.pgo
    @echo "✅ PGO profiles removed"

# =============================================================================
# Building
# =============================================================================

# Build the server binary
build: gen
    @echo "Building binary..."
    go build -o {{binary_name}} ./cmd/server

# Build the background worker binary
build-worker:
    @echo "Building worker..."
    go build -o {{worker_binary_name}} cmd/worker/main.go

# Build the API key generator tool
build-apikey:
    @echo "Building API key generator..."
    go build -o mono-apikey cmd/apikey/main.go

# Generate a new API key
[group('apikey')]
gen-apikey NAME DAYS="":
    #!/usr/bin/env bash
    set -euo pipefail
    just build-apikey
    if [ -z "{{DAYS}}" ]; then
        MONO_STORAGE_DSN="{{dev_storage_dsn}}" ./mono-apikey -name "{{NAME}}"
    else
        MONO_STORAGE_DSN="{{dev_storage_dsn}}" ./mono-apikey -name "{{NAME}}" -days {{DAYS}}
    fi

# [PROD] Generate API key using Docker and .env file
[group('docker-prod')]
docker-gen-apikey NAME DAYS="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "{{DAYS}}" ]; then
        docker compose -f docker-compose.prod.yml run --rm -e NAME="{{NAME}}" apikey -name "{{NAME}}"
    else
        docker compose -f docker-compose.prod.yml run --rm -e NAME="{{NAME}}" -e DAYS={{DAYS}} apikey -name "{{NAME}}" -days {{DAYS}}
    fi

# Build and run server using dev database
run: build
    @echo "Running server..."
    MONO_STORAGE_DSN="{{dev_storage_dsn}}" MONO_HTTP_PORT=8081 MONO_OTEL_ENABLED=false ./{{binary_name}}

# Remove built binaries and PGO profiles
clean:
    @echo "Cleaning up..."
    rm -f mono-server mono-worker mono-apikey timeutc nointerface
    rm -f cmd/server/default.pgo cmd/worker/default.pgo
    rm -f pgo-*.prof merged.pgo cpu.pgo
    @echo "All binaries and PGO profiles removed"

# =============================================================================
# Docker Development
# =============================================================================

# Build the Docker image
[group('docker-dev')]
docker-build:
    @echo "Building Docker image..."
    docker build -t {{docker_image}} .

# Run the Docker container
[group('docker-dev')]
docker-run:
    @echo "Running Docker container..."
    docker run -p 8080:8080 -p 8081:8081 {{docker_image}}

# =============================================================================
# Production Docker Compose Commands
# =============================================================================

# [PROD] Start production services (server, worker, postgres)
[group('docker-prod')]
docker-up:
    @echo "Starting production services..."
    docker compose -f docker-compose.prod.yml up -d
    @echo "✅ Services started. Use 'just docker-logs' to view logs"

# [PROD] Rebuild images and start services (use after git pull)
[group('docker-prod')]
docker-build-up:
    @echo "Rebuilding images and starting services..."
    docker compose -f docker-compose.prod.yml --profile tools up -d --build
    @echo "✅ Services rebuilt and started. Use 'just docker-logs' to view logs"

# [PROD] Rebuild images only (without starting)
[group('docker-prod')]
docker-rebuild:
    @echo "Rebuilding Docker images..."
    docker compose -f docker-compose.prod.yml --profile tools build

# [PROD] Stop production services
[group('docker-prod')]
docker-down:
    @echo "Stopping production services..."
    docker compose -f docker-compose.prod.yml down

# [PROD] Restart production services
[group('docker-prod')]
docker-restart SERVICE="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "{{SERVICE}}" ]; then
        echo "Restarting all services..."
        docker compose -f docker-compose.prod.yml restart
    else
        echo "Restarting {{SERVICE}}..."
        docker compose -f docker-compose.prod.yml restart {{SERVICE}}
    fi

# [PROD] Restart server only (fast, skips migration)
[group('docker-prod')]
docker-restart-server:
    @echo "Restarting server..."
    docker compose -f docker-compose.prod.yml up -d server --force-recreate

# [PROD] Restart workers only (fast, skips migration)
[group('docker-prod')]
docker-restart-workers:
    @echo "Restarting workers..."
    docker compose -f docker-compose.prod.yml up -d {{workers}} --force-recreate

# [PROD] View logs
[group('docker-prod')]
docker-logs SERVICE="":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "{{SERVICE}}" ]; then
        echo "Showing logs for all services (Ctrl+C to exit)..."
        docker compose -f docker-compose.prod.yml logs -f
    else
        echo "Showing logs for {{SERVICE}} (Ctrl+C to exit)..."
        docker compose -f docker-compose.prod.yml logs -f {{SERVICE}}
    fi

# [PROD] View server logs only
[group('docker-prod')]
docker-logs-server:
    @echo "Showing logs for server (Ctrl+C to exit)..."
    docker compose -f docker-compose.prod.yml logs -f server

# [PROD] View worker logs only (all workers)
[group('docker-prod')]
docker-logs-workers:
    @echo "Showing logs for workers (Ctrl+C to exit)..."
    docker compose -f docker-compose.prod.yml logs -f {{workers}}

# [PROD] View postgres logs only
[group('docker-prod')]
docker-logs-postgres:
    @echo "Showing logs for postgres (Ctrl+C to exit)..."
    docker compose -f docker-compose.prod.yml logs -f postgres

# [PROD] Show running containers
[group('docker-prod')]
docker-ps:
    docker compose -f docker-compose.prod.yml ps

# [PROD] Stop and remove all containers and volumes (WARNING: deletes data!)
[group('docker-prod')]
docker-clean:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "⚠️  WARNING: This will delete all data in the database!"
    echo "Press Ctrl+C to cancel, or Enter to continue..."
    read dummy
    echo "Cleaning up production deployment..."
    docker compose -f docker-compose.prod.yml --profile tools down -v
    echo "✅ All containers and volumes removed"

# [PROD] Open shell in server container
[group('docker-prod')]
docker-shell-server:
    docker compose -f docker-compose.prod.yml exec server sh

# [PROD] Open shell in worker container (defaults to worker-1)
[group('docker-prod')]
docker-shell-worker WORKER="worker-1":
    docker compose -f docker-compose.prod.yml exec {{WORKER}} sh

# [PROD] Open PostgreSQL shell
[group('docker-prod')]
docker-shell-postgres:
    #!/usr/bin/env bash
    set -euo pipefail
    USER=${POSTGRES_USER:-mono}
    DB=${POSTGRES_DB:-mono_db}
    docker compose -f docker-compose.prod.yml exec postgres psql -U $USER -d $DB

# [PROD] Check health status of all services
[group('docker-prod')]
docker-health:
    @echo "Checking health status of production services..."
    docker compose -f docker-compose.prod.yml ps

# [PROD] Test server /health endpoint (detects HTTP/HTTPS from SERVER_PORT)
[group('docker-prod')]
docker-health-server:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Testing server health endpoint..."
    if [ -f .env ]; then
        . ./.env 2>/dev/null || true
    fi
    PORT=${SERVER_PORT:-80}
    if [ "${MONO_TLS_ENABLED}" = "true" ]; then
        echo "Testing HTTPS endpoint at https://localhost:$PORT/health"
        curl -f -k -s https://localhost:$PORT/health && echo "\n✅ Server is healthy" || echo "\n❌ Health check failed"
    else
        echo "Testing HTTP endpoint at http://localhost:$PORT/health"
        curl -f -s http://localhost:$PORT/health && echo "\n✅ Server is healthy" || echo "\n❌ Health check failed"
    fi

# =============================================================================
# Migration Image (goose-migrate) - Multi-architecture
# =============================================================================

# Setup Docker buildx for multi-platform builds
[group('docker-migrate')]
docker-buildx-setup:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Setting up Docker buildx builder..."
    if ! docker buildx inspect multiarch-builder > /dev/null 2>&1; then
        docker buildx create --name multiarch-builder --driver docker-container --bootstrap --use
        echo "✅ Created and activated multiarch-builder"
    else
        docker buildx use multiarch-builder
        echo "✅ Activated existing multiarch-builder"
    fi
    docker buildx inspect --bootstrap

# Build goose migration image for current platform only (for local testing)
[group('docker-migrate')]
docker-build-migrate:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Building migration image {{migrate_image}}:{{migrate_image_tag}} for current platform..."
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    docker buildx build \
        -f Dockerfile.migrate \
        --build-arg GOOSE_VERSION={{goose_version}} \
        --build-arg BUILD_DATE=$BUILD_DATE \
        --build-arg GIT_COMMIT=$GIT_COMMIT \
        -t {{migrate_image}}:{{migrate_image_tag}} \
        -t {{migrate_image}}:latest \
        --load \
        .
    echo "✅ Built {{migrate_image}}:{{migrate_image_tag}} for current platform"

# Build and push goose migration image to ghcr.io (multi-arch: amd64, arm64)
[group('docker-migrate')]
docker-push-migrate: docker-buildx-setup
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Building and pushing multi-arch migration image to GitHub Container Registry..."
    echo "Note: Make sure you're logged in with: docker login ghcr.io -u USERNAME"
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    docker buildx build \
        --builder multiarch-builder \
        --platform linux/amd64,linux/arm64 \
        -f Dockerfile.migrate \
        --build-arg GOOSE_VERSION={{goose_version}} \
        --build-arg BUILD_DATE=$BUILD_DATE \
        --build-arg GIT_COMMIT=$GIT_COMMIT \
        -t {{migrate_image}}:{{migrate_image_tag}} \
        -t {{migrate_image}}:latest \
        --push \
        .
    echo "✅ Pushed {{migrate_image}}:{{migrate_image_tag}} and {{migrate_image}}:latest (amd64, arm64)"

# =============================================================================
# Development Database (port 5432)
# =============================================================================

# [DEV DB] Start development database (port 5432)
[group('db-dev')]
db-up:
    @echo "Starting PostgreSQL database..."
    docker compose up -d postgres
    @echo "Waiting for PostgreSQL to be ready..."
    @sleep 3
    @echo "✅ Database ready at {{dev_storage_dsn}}"

# [DEV DB] Stop development database
[group('db-dev')]
db-down:
    @echo "Stopping database containers..."
    docker compose down

# [DEV DB] Stop and remove development database with all data
[group('db-dev')]
db-clean:
    @echo "Cleaning up development database and volumes..."
    docker compose down -v
    @echo "Development database cleaned!"

# =============================================================================
# Database Migrations
# =============================================================================

# Run migrations
[group('db-migrate')]
db-migrate-up DB_URL:
    @echo "Running migrations up..."
    go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations {{db_driver}} "{{DB_URL}}" up

# Rollback migration
[group('db-migrate')]
db-migrate-down DB_URL:
    @echo "Rolling back migration..."
    go run -tags 'no_sqlite' github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations {{db_driver}} "{{DB_URL}}" down

# Create migration
[group('db-migrate')]
db-migrate-create NAME:
    @echo "Creating new migration: {{NAME}}"
    go run github.com/pressly/goose/v3/cmd/goose@latest -dir internal/infrastructure/persistence/postgres/migrations create {{NAME}} sql

# =============================================================================
# Test Database (port 5433)
# =============================================================================

# [TEST DB] Start test database (port 5433)
[group('db-test')]
test-integration-up:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Starting PostgreSQL test database..."
    docker compose -f docker-compose.test.yml up -d
    echo "Waiting for PostgreSQL to be ready..."
    for i in {1..10}; do
        if docker compose -f docker-compose.test.yml exec -T postgres pg_isready -U postgres > /dev/null 2>&1; then
            echo "PostgreSQL is ready!"
            break
        fi
        echo "Waiting for PostgreSQL... ($i/10)"
        sleep 2
    done
    echo "Running migrations..."
    MONO_STORAGE_DSN="{{test_dsn}}" \
        go run github.com/pressly/goose/v3/cmd/goose@latest \
        -dir internal/infrastructure/persistence/postgres/migrations \
        postgres \
        "{{test_dsn}}" \
        up

# [TEST DB] Stop test database
[group('db-test')]
test-integration-down:
    @echo "Stopping PostgreSQL test database..."
    docker compose -f docker-compose.test.yml down

# [TEST DB] Stop test database (data is ephemeral via tmpfs)
[group('db-test')]
test-integration-clean:
    @echo "Stopping PostgreSQL test database..."
    docker compose -f docker-compose.test.yml down
    @echo "Test database stopped (data was in tmpfs, already gone)"

# Run integration tests (requires MONO_STORAGE_DSN env var)
[group('test')]
[no-exit-message]
[private]
test-integration-run:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -z "${MONO_STORAGE_DSN:-}" ]; then
        echo "Error: MONO_STORAGE_DSN is required."
        exit 1
    fi
    go test -p 1 ./tests/integration/... -count=1
    echo "OK: Integration tests"

# [TEST DB] Run integration tests (auto-cleanup before/after)
[group('test')]
[no-exit-message]
test-integration:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'just test-integration-clean >/dev/null 2>&1' EXIT
    docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
    just test-integration-up >/dev/null 2>&1
    MONO_STORAGE_DSN="{{test_dsn}}" go test -p 1 ./tests/integration/... -count=1
    echo "OK: Integration tests"

# [TEST DB] Run HTTP integration tests (auto-cleanup before/after)
[group('test')]
[no-exit-message]
test-integration-http:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'just test-integration-clean >/dev/null 2>&1' EXIT
    docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
    just test-integration-up >/dev/null 2>&1
    MONO_STORAGE_DSN="{{test_dsn}}" go test ./tests/integration/http -count=1
    echo "OK: HTTP integration tests"

# [TEST DB] Run end-to-end tests (auto-cleanup before/after)
[group('test')]
[no-exit-message]
test-e2e:
    #!/usr/bin/env bash
    set -euo pipefail
    trap 'just test-integration-clean >/dev/null 2>&1' EXIT
    docker compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
    just test-integration-up >/dev/null 2>&1
    MONO_STORAGE_DSN="{{test_dsn}}" go test ./tests/e2e -count=1
    echo "OK: E2E tests"

# Run SQL storage tests (requires running database)
[group('test')]
[no-exit-message]
test-sql:
    #!/usr/bin/env bash
    set -euo pipefail
    go test ./internal/infrastructure/persistence/postgres/...
    echo "OK: SQL storage tests"

# Run all tests (unit, race, integration, e2e) - excludes benchmarks
[group('test')]
[no-exit-message]
test-all:
    #!/usr/bin/env bash
    set -euo pipefail
    just test
    just test-race
    just test-integration
    just test-integration-http
    just test-e2e
    echo ""
    echo "OK: All tests passed"

# Run all tests including benchmarks (slow, use sparingly)
[group('test')]
[no-exit-message]
test-all-bench:
    #!/usr/bin/env bash
    set -euo pipefail
    just test-all
    just bench-test
    echo ""
    echo "OK: All tests and benchmarks passed"

# =============================================================================
# Test Database Helpers
# =============================================================================

# [TEST DB] Show test database container status
[group('db-test')]
test-db-status:
    docker compose -f docker-compose.test.yml ps

# Show PostgreSQL test database logs
[group('db-test')]
test-db-logs:
    docker compose -f docker-compose.test.yml logs -f postgres

# Connect to PostgreSQL test database shell
[group('db-test')]
test-db-shell:
    docker compose -f docker-compose.test.yml exec postgres psql -U postgres -d mono_test

# =============================================================================
# Documentation
# =============================================================================

# Sync CLAUDE.md to AGENTS.md
sync-agents:
    @echo "Syncing agent instruction files..."
    cp CLAUDE.md AGENTS.md
    @echo "Synced CLAUDE.md to AGENTS.md"

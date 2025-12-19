# Mono Service

Mono is a simple task management service providing both gRPC and REST APIs. It features recurring tasks, background job processing using a simple job queue, API key authentication, and full observability stack with OpenTelemetry.

## Features

- **Dual API**: gRPC (port 8080) and REST Gateway (port 8081)
- **PostgreSQL Storage Implementation**: ACID-compliant with optimized connection pooling
- **Recurring Tasks**: Template-based task generation with flexible patterns
- **Background Jobs**: Distributed job queue with SKIP LOCKED concurrency
- **API Key Authentication**: bcrypt-secured authentication with gRPC interceptors
- **Observability**: OpenTelemetry tracing, metrics, and structured logging
- **Auto Migrations**: Automatic database schema management with goose
- **Type-Safe SQL**: sqlc-generated data access code
- **Comprehensive Testing**: Unit, integration, e2e, and benchmark tests

## Architecture

### Runtime Components

The Mono service runs **multiple components**:

1. **gRPC Server** (`MONO_GRPC_PORT`): Core application logic serving HTTP/2 requests using Protobuf
2. **HTTP Gateway** (`MONO_HTTP_PORT`): REST/JSON proxy translating HTTP requests to gRPC
3. **Background Worker**: Processes recurring task generation jobs on a schedule

### Codebase Structure (DDD Layered)

```
cmd/
  ├── server/                  # gRPC + REST gateway server
  ├── worker/                  # Background job processor
  └── apikey/                  # API key generation CLI

internal/
  # Domain Layer (pure business logic)
  ├── domain/                  # Domain models, value objects, domain errors

  # Application Layer (use cases)
  ├── application/
  │   ├── auth/                # Authentication service + repository interface
  │   ├── todo/                # Todo business logic + repository interface
  │   └── worker/              # Worker business logic + repository interface

  # Infrastructure Layer (implementations)
  ├── infrastructure/
  │   ├── keygen/              # API key generation and parsing utilities
  │   └── persistence/
  │       └── postgres/        # PostgreSQL repository implementations
  │           ├── migrations/  # Database schema versions (goose)
  │           ├── queries/     # SQL query definitions (sqlc source)
  │           └── sqlcgen/     # Generated type-safe Go code (sqlc)

  # Service Layer (protocol handlers)
  ├── service/                 # Thin gRPC handlers (protocol translation only)

  # Supporting utilities
  ├── recurring/               # Recurrence pattern calculators
  ├── config/                  # Configuration management
  └── env/                     # Environment variable parsing

api/proto/                     # Protobuf definitions (source of truth)
pkg/observability/             # OpenTelemetry setup
tests/
  ├── integration/             # Integration tests with real database
  └── e2e/                     # End-to-end API tests
```

**Layer Responsibilities**:

**1. Domain Layer** (`internal/domain/`)
- Pure business entities and value objects
- Domain errors (ErrNotFound, ErrInvalidID, etc.)
- No dependencies on other layers
- Example: TodoItem, TodoList, RecurringTemplate structs

**2. Application Layer** (`internal/application/`)
- Business logic and use case orchestration
- Defines repository interfaces (Dependency Inversion)
- Coordinates domain objects to fulfill use cases
- Protocol-agnostic (no gRPC, HTTP, or database knowledge)
- Example: CreateItem validates title, generates UUID, calls repository

**3. Infrastructure Layer** (`internal/infrastructure/`)
- Repository implementations (PostgreSQL)
- Database queries, connection management
- Wraps errors with domain error sentinels
- Example: Implements FindListByID, CreateItem using SQL

**4. Service Layer** (`internal/service/`)
- Thin protocol handlers (gRPC in this case)
- Protocol translation only (protobuf ↔ domain models)
- Validates protocol requirements, delegates to application layer
- Maps domain errors to gRPC status codes
- Example: Validates req.Title not empty, calls app.CreateItem, returns proto response

**Key Patterns**:
- **Dependency Inversion**: Application layer defines interfaces, infrastructure implements them
- **Domain Errors**: Infrastructure wraps DB errors with domain sentinels for application layer
- **Layer Flow**: gRPC Handler → Application Service → Repository → Database
- **Code Generation**: Protobuf → Go (buf), SQL → Go (sqlc)

### Design Decisions

**Terminology Note**: "Application Layer" is standard Domain-Driven Design (DDD) terminology.
Also known as "Application Service Layer" or "Use Case Layer" in Clean Architecture.

**Thin Handler Layer**
- gRPC handlers are kept thin (~15-20 lines) with zero business logic
- Each handler follows 4 steps: Validate → Convert → Delegate → Map
- All business logic lives in the application layer
- Field mask handling stays at protocol boundary as optimization

**Application Layer**
- Contains ALL business logic and use case orchestration
- Protocol-agnostic - no knowledge of gRPC, HTTP, or delivery mechanisms
- Same service used by gRPC, REST gateway, CLI, and background workers
- Defines repository interfaces (dependency inversion)

**Benefits**:
- Business logic testable without protocol overhead
- Clear separation of concerns (protocol vs business vs persistence)
- Easy to add new delivery mechanisms (GraphQL, CLI, etc.)
- No circular dependencies between layers

### Database Features

- **PostgreSQL-native types**: UUID, TIMESTAMPTZ, INTERVAL, JSONB
- **UUIDv7 primary keys**: Time-ordered IDs keep B-tree inserts sequential, preventing index fragmentation compared to random UUIDv4.
- **Automatic triggers**: Status history tracking, timestamp updates
- **Connection pooling**: Configurable pool size and lifetime
- **Optimized queries**: N+1 prevention, batch operations
- **Job queue**: SKIP LOCKED for concurrent worker processing

## Quick Start

### Prerequisites

- Go 1.25.5+
- PostgreSQL 15+ (or Docker)
- Buf (for protobuf generation)

### Local Development

```bash
# 1. Start PostgreSQL
make db-up

# 2. Run migrations
DB_URL="postgres://mono:mono_password@localhost:5432/mono_db" make db-migrate-up

# 3. Build and run the server
make build
MONO_POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db" ./mono-server
```

### Using Docker

```bash
# Build and run with Docker Compose
docker-compose up -d
```

**Note**: In containerized environments (Docker, Kubernetes), you may need to set `MONO_GRPC_HOST` to the appropriate service name or IP address for the gateway to connect to the gRPC server. Default is `localhost` which works for single-process deployments.

### Generate API Key

```bash
# Build the API key generator
make build-apikey

# Generate a key (never expires)
POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db" \
  NAME="My Application" make gen-apikey

# Generate a key with 30-day expiration
POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db" \
  NAME="Temporary Key" DAYS=30 make gen-apikey
```

## Configuration

All configuration is via environment variables with `MONO_` prefix:

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MONO_GRPC_PORT` | 8080 | gRPC server port (HTTP/2) |
| `MONO_HTTP_PORT` | 8081 | HTTP gateway port (REST/JSON) |
| `MONO_GRPC_HOST` | localhost | Host for gateway to connect to gRPC server |
| `MONO_ENV` | dev | Environment (`dev`, `prod`) |
| `MONO_STORAGE_TYPE` | postgres | Storage backend (only `postgres` supported) |
| `MONO_POSTGRES_URL` | *required* | PostgreSQL connection string |

### Timeout Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MONO_SHUTDOWN_TIMEOUT` | 10 | Graceful shutdown timeout (seconds) |
| `MONO_HTTP_READ_TIMEOUT` | 5 | HTTP read header timeout (seconds) |
| `MONO_HTTP_WRITE_TIMEOUT` | 10 | HTTP write timeout (seconds) |
| `MONO_HTTP_IDLE_TIMEOUT` | 120 | HTTP idle timeout (seconds) |

### Database Connection Pool

| Variable | Default | Description |
|----------|---------|-------------|
| `MONO_DB_MAX_OPEN_CONNS` | 25 | Maximum open connections |
| `MONO_DB_MAX_IDLE_CONNS` | 5 | Maximum idle connections |
| `MONO_DB_CONN_MAX_LIFETIME` | 300 | Connection max lifetime (seconds) |
| `MONO_DB_CONN_MAX_IDLE_TIME` | 60 | Connection max idle time (seconds) |

### Observability

| Variable | Default | Description |
|----------|---------|-------------|
| `MONO_OTEL_ENABLED` | true | Enable OpenTelemetry |
| `MONO_OTEL_COLLECTOR` | localhost:4317 | OTel collector endpoint |

## API Documentation

The API is documented using OpenAPI v2 (Swagger):
- **File**: [api/openapi/mono.swagger.json](api/openapi/mono.swagger.json)
- View with [Swagger Editor](https://editor.swagger.io/)

### API Usage Examples

#### Create a List

```bash
curl -X POST http://localhost:8081/v1/lists \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"title": "My Todo List"}'
```

#### Add an Item

```bash
curl -X POST http://localhost:8081/v1/lists/{list_id}/items \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Buy groceries",
    "tags": ["shopping", "urgent"],
    "priority": "TASK_PRIORITY_HIGH"
  }'
```

#### Update Item (Partial)

```bash
curl -X PATCH http://localhost:8081/v1/lists/{list_id}/items/{item_id} \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "item": { "status": "TASK_STATUS_DONE" },
    "update_mask": "status"
  }'
```

#### Create Recurring Template

```bash
curl -X POST http://localhost:8081/v1/lists/{list_id}/recurring-templates \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Daily Standup",
    "recurrence_pattern": "RECURRENCE_PATTERN_DAILY",
    "generation_window_days": 30,
    "tags": ["meeting"]
  }'
```

#### Create Item with Recurring Metadata

```bash
curl -X POST http://localhost:8081/v1/lists/{list_id}/items \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Daily Standup - Dec 18",
    "recurring_template_id": "{template_id}",
    "instance_date": "2025-12-18T00:00:00Z",
    "tags": ["recurring", "meeting"]
  }'
```

## Development

### Make Commands

| Command | Description |
|---------|-------------|
| `make` | Generate, test, security check, and build |
| `make gen` | Generate Go code and Swagger docs from Protobuf |
| `make gen-sqlc` | Generate type-safe Go code from SQL queries |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests (starts fresh DB) |
| `make test-all` | Run unit + integration tests |
| `make bench` | Run benchmarks with real database |
| `make bench-test` | Run benchmarks using test database |
| `make security` | Check for vulnerabilities |
| `make lint` | Run linter |
| `make build` | Build the server binary |
| `make build-apikey` | Build the API key generator |
| `make docker-build` | Build Docker image |

### Database Commands

| Command | Description |
|---------|-------------|
| `make db-up` | Start PostgreSQL using Docker |
| `make db-down` | Stop PostgreSQL |
| `make db-migrate-up` | Run migrations (requires DB_URL) |
| `make db-migrate-down` | Rollback last migration |
| `make db-migrate-create` | Create new migration (requires NAME) |
| `make test-integration-up` | Start test database |
| `make test-integration-down` | Stop test database |
| `make test-integration-clean` | Stop and remove test database with volumes |

### Git Hooks

The project uses git hooks for quality assurance:

```bash
# Enable hooks
make setup-hooks
```

The pre-commit hook runs:
1. `make lint` - Code linting
2. `make test` - Unit tests

### Testing

The project has comprehensive test coverage:

#### Unit Tests
```bash
# Run all unit tests
make test

# Run specific package tests
go test -v ./internal/service/...
```

#### Integration Tests
Integration tests use a real PostgreSQL database and verify:
- Database schema and migrations
- CRUD operations
- Triggers and functions
- Job queue with SKIP LOCKED
- Cascade deletes

```bash
# Run integration tests (auto-starts/stops DB)
make test-integration

# Run with cleanup
make test-integration-full
```

#### E2E Tests
End-to-end tests verify the full gRPC API:

```bash
# E2E tests run automatically with test-integration
make test-all
```

#### Benchmarks
Benchmarks use real PostgreSQL to measure actual performance:

```bash
# Run all benchmarks with test database
make bench-test

# Run specific benchmark
BENCHMARK_POSTGRES_URL="postgres://..." \
  go test -bench=BenchmarkCreateList -benchmem ./internal/service/...
```

### API Evolution

The API is defined using **Protobuf** as the Single Source of Truth:

1. **Modify Proto**: Edit `api/proto/mono/v1/mono.proto`
2. **Generate Code**: Run `make gen` to update:
   - Go gRPC stubs
   - Go REST Gateway stubs
   - OpenAPI/Swagger documentation
3. **Implement Logic**: Update service layer
4. **Test**: Run `make test-all`

### SQL Development

```bash
# Modify SQL queries in internal/infrastructure/persistence/postgres/queries/*.sql
make gen-sqlc

# Create a new migration
NAME=add_feature make db-migrate-create

# Test SQL changes
make test-integration
```

## Recurring Tasks

Mono supports flexible recurring task patterns:

### Supported Patterns

- **DAILY**: Every day
- **WEEKLY**: Every week (same day of week)
- **BIWEEKLY**: Every 2 weeks
- **MONTHLY**: Every month (same day of month)
- **QUARTERLY**: Every 3 months
- **YEARLY**: Every year
- **WEEKDAYS**: Monday through Friday

### How It Works

1. **Create Template**: Define a recurring task template with pattern
2. **Background Worker**: Processes templates hourly
3. **Job Queue**: Creates generation jobs for templates
4. **Task Generation**: Worker claims jobs and creates actual tasks
5. **Window Management**: Tracks generation window to avoid duplicates

## Production Deployment

### Environment Variables

```bash
# Required
export MONO_POSTGRES_URL="postgres://user:pass@host:5432/dbname?sslmode=require"

# Recommended for production
export MONO_ENV=prod
export MONO_DB_MAX_OPEN_CONNS=50
export MONO_DB_MAX_IDLE_CONNS=10
export MONO_OTEL_COLLECTOR=otel-collector:4317
export MONO_SHUTDOWN_TIMEOUT=30
```

### Health Checks

The server supports graceful shutdown with configurable timeout. Monitor:
- gRPC server: `localhost:8080` (health check endpoint TBD)
- HTTP gateway: `localhost:8081` (health check endpoint TBD)

### Observability

The service exports:
- **Traces**: Via OpenTelemetry to configured collector
- **Metrics**: Via OpenTelemetry (request counts, latencies, errors)
- **Logs**: Structured JSON logs via slog

### Database Maintenance

```bash
# Backup database
pg_dump $MONO_POSTGRES_URL > backup.sql

# Run migrations on production
DB_URL=$MONO_POSTGRES_URL make db-migrate-up

# Monitor connection pool
# Check MONO_DB_MAX_OPEN_CONNS and adjust based on load
```

## Security

- **API Keys**: Secured with bcrypt cost 14
- **SQL Injection**: Protected via parameterized queries (sqlc)
- **Vulnerabilities**: Regular scanning with `make security`
- **Authentication**: gRPC interceptor validates all requests
- **TLS**: Configure with gRPC TLS credentials (not shown in quick start)

## Performance

Benchmark results on Apple M3 Pro (for reference):

| Operation | Throughput | Latency |
|-----------|-----------|---------|
| CreateList | ~600 ops/sec | ~1.6ms |
| CreateItem | Variable | ~2-3ms |
| ListTasks (100) | ~1500 ops/sec | ~660μs |
| ListTasks (1K) | ~470 ops/sec | ~2.1ms |
| ListTasks (10K) | ~47 ops/sec | ~21ms |

*Run `make bench-test` for your environment*

## License

[Your License Here]

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run `make` to verify tests pass
5. Submit a pull request

Ensure:
- All tests pass: `make test-all`
- Code is formatted: `make lint`
- Benchmarks don't regress significantly
- API changes update Protobuf definitions

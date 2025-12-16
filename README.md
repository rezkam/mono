# Mono Service

Mono is a To-Do list application service providing both gRPC and REST APIs. It supports multiple storage backends (File System, Google Cloud Storage, PostgreSQL, and SQLite) and features OpenTelemetry integration for observability.

## Features

- **Dual API**: gRPC (port 8080) and REST Gateway (port 8081).
- **Storage Backends**:
    - `fs`: Local JSON file storage (default).
    - `gcs`: Google Cloud Storage.
    - `postgres`: PostgreSQL database (ACID-compliant, scalable).
    - `sqlite`: SQLite database (embedded, zero-config).
- **Observability**: OpenTelemetry tracing and metrics support.
- **API Documentation**: Auto-generated Swagger/OpenAPI documentation.
- **Database Migrations**: Automatic schema management with goose.
- **Type-Safe SQL**: sqlc-generated data access code.

## API Documentation

The API is documented using OpenAPI v2 (Swagger).

*   **File**: [api/openapi/mono.swagger.json](api/openapi/mono.swagger.json)

You can view this file using any Swagger UI viewer (e.g., [Swagger Editor](https://editor.swagger.io/)) by importing the JSON file.

## Quick Start

### Local Run (FileSystem - Default)

```bash
# Build & Run via Make
make run
```

Or manually:

```bash
go build -o mono-server cmd/server/main.go
./mono-server
```

The server will store data in `./mono-data` by default.

### Local Run (SQLite)

```bash
# Build the server
make build

# Run with SQLite storage
MONO_STORAGE_TYPE=sqlite MONO_SQLITE_PATH=./mono-data/mono.db ./mono-server
```

### Local Run (PostgreSQL)

```bash
# Start PostgreSQL using Docker
make db-up

# Run the server
MONO_STORAGE_TYPE=postgres \
MONO_POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db" \
./mono-server
```

## Architecture

The Mono service runs **two servers** simultaneously:

1.  **gRPC Server** (`MONO_GRPC_PORT`): The core application logic serving HTTP/2 requests using the Protobuf protocol. This is for internal high-performance communication.
2.  **HTTP Gateway** (`MONO_HTTP_PORT`): A reverse-proxy that accepts standard JSON/REST requests (HTTP/1.1) and translates them into gRPC calls to the local gRPC server. This allows external clients to use the API easily.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MONO_GRPC_PORT` | 8080 | gRPC server port (HTTP/2) |
| `MONO_HTTP_PORT` | 8081 | HTTP gateway port (REST/JSON) |
| `MONO_ENV` | dev | Environment (`dev`, `prod`) |
| `MONO_STORAGE_TYPE` | fs | Storage backend (`fs`, `gcs`, `postgres`, `sqlite`) |
| `MONO_FS_DIR` | ./mono-data | Directory for fs storage (Required if storage=fs) |
| `MONO_GCS_BUCKET` | "" | GCS bucket name (Required if storage=gcs) |
| `MONO_POSTGRES_URL` | "" | PostgreSQL connection string (Required if storage=postgres) |
| `MONO_SQLITE_PATH` | ./mono-data/mono.db | SQLite database path (Required if storage=sqlite) |
| `MONO_DB_MAX_OPEN_CONNS` | 25 | Maximum open database connections |
| `MONO_DB_MAX_IDLE_CONNS` | 5 | Maximum idle database connections |
| `MONO_DB_CONN_MAX_LIFETIME` | 300 | Connection max lifetime in seconds |
| `MONO_DB_CONN_MAX_IDLE_TIME` | 60 | Connection max idle time in seconds |
| `MONO_OTEL_ENABLED` | true | Enable OpenTelemetry |
| `MONO_OTEL_COLLECTOR` | localhost:4317 | OTel collector endpoint |

## API Evolution

The API is defined using **Protobuf** as the Single Source of Truth. The workflow for modifying the API is:

1.  **Modify Proto**: Edit `api/proto/monov1/mono.proto`.
2.  **Generate Code**: Run `make gen`. This updates:
    *   Go gRPC stubs.
    *   Go REST Gateway stubs.
    *   OpenAPI/Swagger documentation (`api/openapi/mono.swagger.json`).
3.  **Implement Logic**: Update `internal/service/mono.go` to match the new interface.
4.  **Verify**: Run `make test` to ensure changes are correct and compatible.

## API Usage Patterns

### Partial Updates (FieldMask)

The `UpdateItem` endpoint supports **Partial Updates** via `update_mask`, following standard Google API patterns (AIP-134). This allows you to update specific fields without affecting others.

**Example: Update only the Title**

```bash
curl -X PATCH http://localhost:8081/v1/lists/{list_id}/items/{item.id} \
  -H "Content-Type: application/json" \
  -d '{
    "item": { "title": "New Title" },
    "update_mask": "title"
  }'
```

**Example: Mark as Completed**

```bash
curl -X PATCH http://localhost:8081/v1/lists/{list_id}/items/{item.id} \
  -H "Content-Type: application/json" \
  -d '{
    "item": { "completed": true },
    "update_mask": "completed"
  }'
```

If `update_mask` is omitted, the behavior may vary (often full replace), so it is highly recommended to always provide it for clarity.

## Development

### Prerequisites

*   Go 1.25.5+
*   Buf
*   Protoc plugins (install via `make gen` dependencies usually, or manually)

### Commands

*   `make`: Generate, test, security check, and build.
*   `make gen`: Generate Go code and Swagger docs from Protobuf.
*   `make gen-sqlc`: Generate type-safe Go code from SQL queries.
*   `make test`: Run unit, integration, and E2E tests.
*   `make test-sql`: Run SQL storage integration tests.
*   `make security`: Check for vulnerabilities.
*   `make docker-build`: Build Docker image.
*   `make db-up`: Start PostgreSQL database using Docker.
*   `make db-down`: Stop database containers.

### SQL Storage Development

For detailed information about SQL storage, migrations, and database management, see:
- [SQL Storage Documentation](docs/SQL_STORAGE.md)
- [Database Migrations Guide](docs/MIGRATIONS.md)

Quick reference for common SQL operations:

```bash
# Generate sqlc code after modifying queries
make gen-sqlc

# Create a new migration
NAME=add_priority_field make db-migrate-create

# Start PostgreSQL for testing
make db-up

# Run SQL storage tests
make test-sql

# Stop database
make db-down
```

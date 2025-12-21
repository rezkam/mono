# Mono Service

Mono is a simple task management service providing an HTTP/REST API with recurring tasks, background job processing, API key authentication, and full observability.

## Features

- **REST API**: HTTP/JSON API
- **Database Storage**: ACID-compliant with optimized connection pooling
- **Recurring Tasks**: Template-based task generation with flexible patterns
- **Background Jobs**: Distributed job queue with concurrent processing
- **API Key Authentication**: Secure authentication with HTTP middleware
- **Observability**: Tracing, metrics, and structured logging
- **Auto Migrations**: Automatic database schema management
- **Type-Safe SQL**: Generated data access code
- **Comprehensive Testing**: Unit, integration, e2e, and benchmark tests

## Architecture

### Runtime Components

The Mono service runs **multiple components**:

1. **HTTP Server**: REST/JSON API serving application logic
2. **Background Worker**: Processes recurring task generation jobs on a schedule

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
- Protocol-agnostic (no HTTP or database knowledge)
- Example: CreateItem validates title, generates UUID, calls repository

**3. Infrastructure Layer** (`internal/infrastructure/`)
- Repository implementations
- Database queries, connection management
- Wraps errors with domain error sentinels
- Example: Implements FindListByID, CreateItem using SQL

**4. HTTP Layer** (`internal/http/`)
- HTTP handlers and routing
- Protocol translation (JSON ↔ domain models)
- Validates HTTP requirements, delegates to application layer
- Maps domain errors to HTTP status codes
- Example: Validates request body, calls app.CreateItem, returns JSON response

**Key Patterns**:
- **Dependency Inversion**: Application layer defines interfaces, infrastructure implements them
- **Domain Errors**: Infrastructure wraps DB errors with domain sentinels for application layer
- **Layer Flow**: HTTP Handler → Application Service → Repository → Database
- **Code Generation**: OpenAPI → Go, SQL → Go

### Design Decisions

**Terminology Note**: "Application Layer" is standard Domain-Driven Design (DDD) terminology.

**HTTP Layer**
- Handles protocol translation and validation
- Delegates business logic to application layer
- Minimal logic focused on HTTP concerns

**Application Layer**
- Contains business logic and use case orchestration
- Protocol-agnostic
- Used by HTTP API, CLI, and background workers
- Defines repository interfaces (dependency inversion)

**Benefits**:
- Business logic testable without protocol overhead
- Clear separation of concerns
- No circular dependencies between layers

### Database Features

- **Time-ordered IDs**: Sequential inserts prevent index fragmentation
- **Automatic triggers**: Status history tracking, timestamp updates
- **Connection pooling**: Configurable pool size and lifetime
- **Optimized queries**: Batch operations and efficient joins
- **Job queue**: Concurrent worker processing

### Configuration Pattern

Each binary loads only the configuration it needs:
- **Server**: Database, auth, pagination, observability
- **Worker**: Database, operation timeout
- **API Key Tool**: Database, API key settings
- **Tests**: Database configuration

## API Documentation

The API is documented using OpenAPI v2 (Swagger):
- **File**: [api/openapi/mono.swagger.json](api/openapi/mono.swagger.json)
- View with [Swagger Editor](https://editor.swagger.io/)

### Git Hooks

The project uses git hooks for quality assurance:

```bash
# Enable hooks
make setup-hooks
```

### Testing

The project has comprehensive test coverage across multiple levels:


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

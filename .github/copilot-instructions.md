# Mono Service - Agent Instructions

> This file is the source of truth. Run `make sync-agents` to copy to AGENTS.md and .github/copilot-instructions.md

## Project Overview

**Mono** is a production-ready task management service built with Go, providing both gRPC and REST APIs.

### Tech Stack
- **Go**: 1.25.5+
- **PostgreSQL**: 18+ (with uuidv7 support)
- **Protocol Buffers**: v3 with buf for code generation
- **Database Tools**: sqlc for type-safe queries, goose for migrations
- **Testing**: testify for assertions, integration tests with real PostgreSQL
- **Observability**: OpenTelemetry for tracing and metrics

### Architecture (DDD Layered)

```
cmd/                           # Binary entrypoints (server, worker, apikey)
internal/
  ├── domain/                  # Domain models, value objects, errors
  ├── application/             # Use cases with consumer-defined interfaces
  │   ├── auth/                # Authentication service
  │   ├── todo/                # Todo service + Repository interface
  │   └── worker/              # Worker service + Repository interface
  ├── infrastructure/
  │   └── persistence/postgres/  # Concrete repository implementations
  ├── service/                 # gRPC handlers (imports domain, uses app interfaces)
  ├── storage/sql/             # SQL infrastructure
  │   ├── migrations/          # Database schema versions (goose)
  │   ├── queries/             # SQL query definitions (sqlc source)
  │   └── sqlcgen/             # Generated type-safe Go code
  ├── auth/                    # API key generation utilities
  ├── recurring/               # Recurrence calculation logic
  └── config/                  # Configuration management
api/proto/                     # Protobuf definitions (source of truth)
tests/
  ├── integration/             # Integration tests with real database
  └── e2e/                     # End-to-end tests
```

### Key Patterns
- **DDD Layered Architecture**: `cmd/` → `internal/service/` → `internal/application/` → `internal/infrastructure/`
- **Dual API**: gRPC server (8080) with HTTP/REST gateway (8081)
- **Consumer Interfaces**: Each application service defines its own repository interface (Dependency Inversion Principle)
- **Code Generation**: Protobuf → Go (gRPC + HTTP gateway), SQL → Go (sqlc)

## Commands

### Development Workflow
| Command | Description |
|---------|-------------|
| `make build` | Build all binaries |
| `make lint` | Run `go vet ./...` |
| `make test` | Run all unit tests |
| `make test-integration` | Run integration tests (auto-starts DB) |
| `make test-all` | Run unit + integration tests |
| `go test -v ./internal/service -run TestName` | Run specific test |

### Code Generation
| Command | Description |
|---------|-------------|
| `make gen` | Regenerate gRPC/HTTP code from proto |
| `make gen-sqlc` | Regenerate type-safe DB code from SQL |

**Never** manually edit generated files (`*.pb.go`, `sqlcgen/*.go`)

### Database
| Command | Description |
|---------|-------------|
| `make db-up` | Start PostgreSQL (Docker) |
| `make db-migrate-up` | Run migrations |
| `make db-migrate-create NAME=xyz` | Create new migration |

## DO: Required Patterns

### UUID Generation
Always use **UUIDv7** (time-ordered) for primary keys:
```go
// ✅ CORRECT
idObj, err := uuid.NewV7()
if err != nil {
    return status.Errorf(codes.Internal, "failed to generate id: %v", err)
}
id := idObj.String()

// ❌ WRONG - Random UUIDs cause index fragmentation
id := uuid.New().String()  // This is UUIDv4
```

### Repository Operations
For individual item operations, use dedicated methods to **preserve audit trails**:
```go
// ✅ CORRECT - Preserves status history via database triggers
s.repo.CreateItem(ctx, listID, &item)
s.repo.UpdateItem(ctx, &item)

// ❌ WRONG - UpdateList deletes and recreates items, wiping history
list.AddItem(item)
s.repo.UpdateList(ctx, list)
```

### Field Mask Support in Update RPCs
Always respect `update_mask` for partial updates:
```go
// ✅ CORRECT - See internal/service/recurring.go UpdateRecurringTemplate
if req.UpdateMask == nil || len(req.UpdateMask.Paths) == 0 {
    // Update all fields
} else {
    // Update only specified fields
    for _, path := range req.UpdateMask.Paths {
        switch path {
        case "title":
            existing.Title = req.Item.Title
        // ... other fields
        }
    }
}

// ❌ WRONG - Blindly overwriting all fields wipes unspecified fields to zero values
```

### Error Handling
```go
// ✅ CORRECT - Wrap errors with context, use domain errors
if err := s.repo.CreateList(ctx, list); err != nil {
    if errors.Is(err, domain.ErrListNotFound) {
        return nil, status.Error(codes.NotFound, "list not found")
    }
    return nil, status.Errorf(codes.Internal, "failed to create list: %v", err)
}

// ❌ WRONG - Raw errors without context
if err != nil {
    return nil, err
}

// ❌ WRONG - Panics in production code
if err != nil {
    panic(err)
}
```

### Database Migrations
```sql
-- ✅ CORRECT - Use uuidv7() for all primary keys
CREATE TABLE example (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    ...
);

-- ❌ WRONG - gen_random_uuid() is deprecated
CREATE TABLE example (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ...
);
```

## DON'T: Anti-Patterns to Avoid

### Don't Edit Generated Code
Files to **never** manually edit:
- `api/proto/mono/v1/*.pb.go` (protobuf generated)
- `internal/storage/sql/sqlcgen/*.go` (sqlc generated)

If you need changes, edit the source (`.proto` or `.sql`) and regenerate.

### Don't Use UpdateList for Item Operations
`UpdateList` deletes all items and recreates them, wiping status history:
```go
// ❌ WRONG - Causes data loss
list.Items = append(list.Items, newItem)
storage.UpdateList(ctx, list)

// ✅ CORRECT
storage.CreateItem(ctx, listID, &newItem)
```

**Note**: `UpdateList` is acceptable only for test cleanup where history loss is expected.

### Don't Skip Field Mask Validation
Partial updates must respect the provided field mask to avoid data loss.

### Don't Modify Schema Without Migration
Always create a migration file:
```bash
make db-migrate-create NAME=add_column_xyz
# Edit the generated file in internal/storage/sql/migrations/
```

## Exemplary Files (Copy These Patterns)

### Service Layer with Field Mask
**Copy**: `internal/service/recurring.go` - `UpdateRecurringTemplate` method

### Item Operations Preserving History
**Copy**: `internal/service/mono.go` - `CreateItem` and `UpdateItem` methods

### Integration Tests with Database
**Copy**: `tests/integration/item_status_history_test.go`

### Repository Implementation
**Copy**: `internal/infrastructure/persistence/postgres/todo_repository.go`

### Consumer Interface Definition
**Copy**: `internal/application/todo/repository.go`

## Code Review Learnings

1. **Status History Preservation**: Always use `CreateItem`/`UpdateItem` for item operations. `UpdateList` wipes history via CASCADE DELETE.

2. **Field Mask Implementation**: Partial updates require checking `update_mask.paths` to avoid data loss. See `UpdateRecurringTemplate` for reference.

3. **UUID Consistency**: Database defaults (`uuidv7()`) and Go code (`uuid.NewV7()`) must match. Check both migration files and service layer.

4. **Call Site Analysis**: Before flagging "unused" methods, verify actual usage. Some methods are intentionally used only in tests (e.g., `UpdateList` in benchmarks).

5. **Integration Test Value**: Tests with real databases (like status history suite) are the source of truth for behavior verification.

6. **Domain Error Handling**: Infrastructure layer must wrap errors with domain errors (`domain.ErrListNotFound`, etc.) so service layer can map to proper gRPC status codes.

7. **Consumer Interfaces (DIP)**: Each application service defines its own repository interface. Infrastructure implements these interfaces. Service layer depends only on domain types.

## Interface Documentation Best Practices

Based on Go standard library patterns (`database/sql`, `io`, `sync/atomic`) and [Google's Go Style Guide](https://google.github.io/styleguide/go/best-practices.html):

### What to Document (Part of Contract)

**1. Concurrency/Safety Guarantees**
```go
// ✅ DO - Behavioral guarantee callers depend on
// ClaimNextGenerationJob atomically claims the next pending job.
// Returns empty string if no jobs available.
ClaimNextGenerationJob(ctx context.Context) (string, error)

// ❌ DON'T - Implementation detail
// ClaimNextGenerationJob atomically claims the next pending job using SKIP LOCKED.
```

**Standard library examples:**
- `database/sql.DB`: "It's safe for concurrent use by multiple goroutines"
- `io.ReaderAt`: "Clients of ReadAt can execute parallel ReadAt calls"
- `sync/atomic`: Documents "sequentially consistent" memory guarantees

**2. Error Conditions and Return Values**
```go
// ✅ DO - What errors mean
// FindByID retrieves an item by ID.
// Returns domain.ErrNotFound if item doesn't exist.
FindByID(ctx context.Context, id string) (*Item, error)

// ❌ DON'T - Too verbose, restates signature
// FindByID retrieves an item by ID.
// Parameters: ctx is context, id is the item ID.
// Returns: Item pointer and error.
```

**3. Behavioral Contracts (Non-Obvious Requirements)**
```go
// ✅ DO - Critical caller responsibility
// UpdateItem updates an existing todo item.
// Validates that the item belongs to the specified list (prevents cross-list updates).
UpdateItem(ctx context.Context, listID string, item *TodoItem) error
```

### What NOT to Document (Implementation Details)

**1. Implementation Mechanisms**
```go
// ❌ DON'T - Database implementation detail
// CreateItem creates a new todo item.
// Preserves status history via database triggers.

// ✅ DO - Just the contract
// CreateItem creates a new todo item.
CreateItem(ctx context.Context, listID string, item *TodoItem) error
```

**2. Logging/Monitoring Details**
```go
// ❌ DON'T - Non-functional implementation concern
// UpdateLastUsed updates the last used timestamp.
// Failures are logged but don't block authentication.

// ✅ DO - Just the operation
// UpdateLastUsed updates the last used timestamp.
UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error
```

**3. Usage Examples/Callers**
```go
// ❌ DON'T - Maintenance burden, IDE shows this
// FindByShortToken retrieves an API key by its short token.
// Used by: validateAPIKey() during gRPC interceptor authentication

// ✅ DO - Just what it does
// FindByShortToken retrieves an API key by its short token.
FindByShortToken(ctx context.Context, shortToken string) (*APIKey, error)
```

**4. Architecture/Layer References**
```go
// ❌ DON'T - Couples to current architecture
// Repository defines storage operations.
// This interface is defined by the application layer and implemented by the infrastructure layer.

// ✅ DO - Architecture-agnostic
// Repository defines storage operations for todo management.
type Repository interface {
```

**5. Performance Implementation**
```go
// ❌ DON'T - How filtering is implemented
// FindItems searches for items with filtering, sorting, and pagination.
// All filtering happens at the database level for performance.

// ✅ DO - What it does
// FindItems searches for items with filtering, sorting, and pagination.
FindItems(ctx context.Context, params ListTasksParams) (*PagedResult, error)
```

**6. Historical Context**
```go
// ❌ DON'T - References past states
// UpdateItem updates an item.
// Previously this didn't check list ownership, now it does.
// Fixed in PR #123 to prevent security issue.

// ✅ DO - Current behavior only
// UpdateItem updates an existing todo item.
// Validates that the item belongs to the specified list.
UpdateItem(ctx context.Context, listID string, item *TodoItem) error
```

### Key Principles

1. **Document behavioral guarantees** that callers depend on (concurrency, atomicity, error conditions)
2. **Don't document how you achieve those guarantees** (SKIP LOCKED, triggers, logging, layers)
3. **Assume implementation is correct** - don't prove it in comments
4. **Focus on error-prone or non-obvious aspects** - not obvious details
5. **Keep comments maintainable** - avoid references to callers, layers, past bugs

**Source:** [Google Go Style Guide](https://google.github.io/styleguide/go/best-practices.html) states: *"Whether an API is safe for use by multiple goroutines is part of its contract"* but implementation details like logging are not.

## Safety Boundaries

### Always Do
- Run `make lint` before committing
- Add tests for new features (TDD preferred)
- Use transactions for multi-step database operations
- Validate proto messages with buf

### Ask First
- Adding new gRPC methods (needs proto review)
- Changing database schema (needs migration strategy)
- Adding new dependencies
- Modifying authentication/authorization logic

### Never Do
- Push without running tests (`make test`)
- Manually edit generated code
- Use raw SQL queries (use sqlc)
- Panic in production code
- Skip field mask handling in update operations

## Quick Reference

```bash
# Fix after proto changes
make gen && make build && make test

# Fix after SQL changes
make gen-sqlc && make build && make test

# Run integration tests
make test-integration

# Generate API key
POSTGRES_URL="..." NAME="MyApp" make gen-apikey
```

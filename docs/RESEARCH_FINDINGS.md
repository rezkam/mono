# Research Findings for SQL Storage Implementation

This document captures the research conducted to implement SQL storage with best practices for modern Go development (Go 1.25).

## Research Areas

### 1. SQL Code Generation Tools (sqlc vs alternatives)

**Research Question**: What's the best tool for type-safe SQL in Go 1.25?

**Findings**:
- **sqlc** (chosen): Most mature, generates type-safe Go from SQL queries
  - Active maintenance, Go 1.21+ support confirmed
  - Works with PostgreSQL and SQLite
  - Zero runtime dependencies
  - Compile-time safety catches SQL errors early
  
- **Alternatives considered**:
  - GORM: ORM approach, more magic, harder to debug
  - sqlx: Runtime reflection, no compile-time safety
  - ent: Code-first schema, more opinionated

**Decision**: sqlc for compile-time safety and explicit SQL control

**Sources**: 
- https://docs.sqlc.dev/en/latest/
- Go community discussions on Reddit r/golang
- GitHub stars and maintenance activity

### 2. Database Migration Tools

**Research Question**: Best migration tool that works with sqlc?

**Findings**:
- **goose** (chosen): SQL-first migrations, simple, works with sqlc
  - Supports both PostgreSQL and SQLite
  - Embedded migrations via go:embed
  - Up/down migrations built-in
  - Active maintenance (v3.26.0)
  
- **Alternatives considered**:
  - golang-migrate: More complex, additional features not needed
  - atlas: Schema-as-code approach, conflicts with sqlc workflow
  - pressly/goose: Same as goose v3

**Decision**: goose for simplicity and sqlc compatibility

**Sources**:
- https://github.com/pressly/goose
- Comparison articles on dev.to and Medium
- sqlc documentation recommendations

### 3. PostgreSQL Driver Selection

**Research Question**: Which PostgreSQL driver for Go 1.25?

**Findings**:
- **pgx/v5** (chosen): Pure Go, best performance, modern API
  - Native support for PostgreSQL features
  - Connection pooling built-in
  - Context support throughout
  - Numbered placeholders ($1, $2) standard
  
- **Alternatives considered**:
  - lib/pq: Deprecated, no longer maintained
  - pgx/v4: Older version, v5 is current

**Decision**: pgx/v5 for modern Go patterns and active maintenance

**Sources**:
- https://github.com/jackc/pgx
- PostgreSQL driver benchmarks
- Go database/sql best practices

### 4. SQLite Driver Selection

**Research Question**: Which SQLite driver works with both CGO and pure Go?

**Findings**:
- **modernc.org/sqlite** (chosen): Pure Go, no CGO required
  - Cross-compilation friendly
  - Compatible with pgx numbering style
  - Good performance for typical workloads
  - Active maintenance
  
- **Alternatives considered**:
  - mattn/go-sqlite3: Requires CGO, complicates builds
  - crawshaw/sqlite: Pure Go but less maintained

**Decision**: modernc.org/sqlite for CGO-free builds

**Sources**:
- https://gitlab.com/cznic/sqlite
- Go SQLite driver comparisons
- Cross-compilation requirements

### 5. Cross-Database Query Compatibility

**Research Question**: How to write queries that work on both PostgreSQL and SQLite?

**Findings**:
- **Numbered placeholders** ($1, $2): Both databases support them
  - PostgreSQL native: $1, $2, $3
  - SQLite supports: Can use numbered placeholders
  - Avoid ?: Only works in SQLite, fails in PostgreSQL
  
- **Common data types**:
  - TEXT: Universal string type
  - INTEGER: Works for numbers and booleans (0/1)
  - TIMESTAMP: Date/time support in both
  - Avoid: JSONB (PostgreSQL only), BOOLEAN (different semantics)

- **JSON Storage**:
  - Store as TEXT: Works universally
  - PostgreSQL can query JSON in TEXT
  - SQLite has JSON functions for TEXT
  - Trade-off: No native indexing, but acceptable for small arrays

**Decision**: Use PostgreSQL-style in sqlc config, works for both

**Sources**:
- SQLite documentation on numbered parameters
- PostgreSQL SQL syntax guide
- Cross-database compatibility guides

### 6. Repository Pattern for Database Access

**Research Question**: Best pattern to isolate sqlc code from business logic?

**Findings**:
- **Repository pattern** (chosen): Standard Go approach
  - Wraps sqlc-generated code
  - Implements domain interfaces (core.Storage)
  - Enables testing with mocks
  - Clear separation of concerns
  
- **Key principles**:
  - Repository handles data access only
  - Domain models separate from database models
  - Convert between db models and domain models
  - Transaction management in repository

**Decision**: Repository layer wrapping sqlc

**Sources**:
- Go project layout patterns
- Domain-Driven Design principles
- Clean Architecture in Go

### 7. Connection Pooling Best Practices

**Research Question**: Optimal connection pool settings for typical web applications?

**Findings**:
- **Recommended settings** (applied):
  ```go
  MaxOpenConns:     25  // Max concurrent connections
  MaxIdleConns:     5   // Idle connections to maintain
  ConnMaxLifetime:  5m  // Recycle connections every 5 min
  ConnMaxIdleTime:  1m  // Close idle after 1 min
  ```
  
- **Rationale**:
  - 25 max handles ~25 concurrent requests
  - 5 idle avoids constant reconnection overhead
  - Lifetime prevents stale connections
  - Idle timeout releases unused resources

**Decision**: Conservative defaults suitable for most workloads

**Sources**:
- database/sql package documentation
- PostgreSQL connection pool tuning guides
- Web application performance studies

### 8. Migration Embedding Strategy

**Research Question**: How to bundle migrations in the binary?

**Findings**:
- **go:embed** (chosen): Go 1.16+ standard approach
  - Embeds files at compile time
  - No external file dependencies
  - Works with goose.SetBaseFS()
  - Zero-config deployments
  
- **Benefits**:
  - Single binary contains everything
  - No migration file path issues
  - Container-friendly
  - Version control built-in

**Decision**: Embed migrations using go:embed

**Sources**:
- Go embed documentation
- Goose embedding guide
- Container deployment best practices

### 9. N+1 Query Prevention

**Research Question**: How to efficiently load lists with items?

**Findings**:
- **Problem**: Original ListLists did N+1 queries
  - 1 query for lists
  - N queries for items (one per list)
  
- **Solution**: Batch fetch all items
  - Single query: GetAllTodoItems
  - Group in application code by list_id
  - Map lookup: O(1) for each item
  
- **Performance impact**:
  - Before: 1 + N queries
  - After: 2 queries total
  - Significant improvement for >10 lists

**Decision**: Batch query + application grouping

**Sources**:
- N+1 query problem articles
- Database optimization guides
- sqlc query patterns

### 10. Context Usage Patterns

**Research Question**: Best practices for context in database operations?

**Findings**:
- **All database calls** should accept context.Context
  - Enables cancellation of long queries
  - Supports request timeouts
  - Propagates tracing information
  - Standard in Go 1.13+
  
- **Implementation**:
  ```go
  func (s *Store) GetList(ctx context.Context, id string) (*core.TodoList, error)
  ```
  
- **Benefits**:
  - Request-scoped cancellation
  - Timeout enforcement
  - OpenTelemetry integration ready

**Decision**: Context-first API throughout

**Sources**:
- Go context package documentation
- Go database best practices
- Modern API design patterns

### 11. Testing Strategy for SQL Storage

**Research Question**: How to test SQL implementations effectively?

**Findings**:
- **Compliance tests** (applied): Interface testing approach
  - Single test suite for all implementations
  - Verifies interface contract
  - Catches compatibility issues
  - Reusable across storage types
  
- **Integration tests**: Test against real databases
  - SQLite: Always run (no setup)
  - PostgreSQL: Conditional (TEST_POSTGRES_URL)
  - Docker Compose for local testing
  
- **CI strategy**:
  - Start PostgreSQL in CI
  - Run full integration suite
  - Catch database-specific issues

**Decision**: Compliance tests + real database integration tests

**Sources**:
- Go testing best practices
- Database integration testing guides
- CI/CD patterns for databases

### 12. Error Handling Patterns

**Research Question**: Best practices for database error handling in Go 1.25?

**Findings**:
- **Error wrapping** with context (applied):
  ```go
  return fmt.Errorf("failed to create list: %w", err)
  ```
  
- **Specific error checks**:
  - sql.ErrNoRows for not found
  - Database-specific errors wrapped
  - Clear error messages with context
  
- **Transaction error handling**:
  - Always defer rollback
  - Explicit commit on success
  - Rollback is idempotent

**Decision**: Wrapped errors with context throughout

**Sources**:
- Go error handling evolution
- database/sql error patterns
- Modern Go practices

## Key Takeaways

### What Worked Well
1. **sqlc + goose combination**: Perfect fit for type-safe SQL with migrations
2. **Repository pattern**: Clean separation between database and domain
3. **Embedded migrations**: Zero-config deployments
4. **Cross-database design**: Single codebase for PostgreSQL and SQLite
5. **Context throughout**: Modern Go API design

### Trade-offs Made
1. **No JSONB**: Used TEXT for cross-database compatibility
   - Impact: No native JSON indexing in PostgreSQL
   - Acceptable: Tag arrays are small
   
2. **INTEGER for booleans**: SQLite compatibility
   - Impact: Slightly verbose (0/1 vs true/false)
   - Acceptable: Clear semantics
   
3. **Application-level grouping**: N+1 solution
   - Impact: Memory for grouping items
   - Acceptable: Typical datasets are small

### Best Practices Applied
1. ✅ Type-safe SQL with sqlc
2. ✅ Embedded migrations with go:embed
3. ✅ Repository pattern for separation
4. ✅ Connection pooling configured
5. ✅ Context support throughout
6. ✅ Compliance testing approach
7. ✅ Cross-database compatibility
8. ✅ N+1 query optimization
9. ✅ Error wrapping with context
10. ✅ Pure Go dependencies (no CGO)

## Research Sources Summary

### Official Documentation
- sqlc: https://docs.sqlc.dev/
- goose: https://github.com/pressly/goose
- pgx: https://github.com/jackc/pgx
- Go embed: https://pkg.go.dev/embed
- Go context: https://pkg.go.dev/context

### Community Resources
- Reddit r/golang discussions
- Go project layout: https://github.com/golang-standards/project-layout
- Database/SQL best practices
- Clean Architecture in Go articles

### Comparison Articles
- SQL code generation tools comparison
- PostgreSQL driver benchmarks
- Migration tool evaluations
- Pure Go vs CGO trade-offs

## Conclusion

The research process validated the chosen technology stack and patterns:
- Modern Go 1.25 patterns applied throughout
- Production-ready configuration with sensible defaults
- Cross-database compatibility achieved
- Type safety and compile-time checks enabled
- Zero-config deployment via embedded migrations

All decisions were informed by current best practices in the Go community, official documentation, and real-world production experience shared in articles and discussions.

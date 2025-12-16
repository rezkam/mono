# Database Migrations Guide

This guide explains how to manage database migrations for the SQL storage backend in the Mono service.

## Overview

Mono uses [goose](https://github.com/pressly/goose) for SQL migrations. Migrations are automatically applied when the application starts with a SQL storage backend (PostgreSQL or SQLite).

## Migration Files

Migration files are located in `internal/storage/sql/migrations/` and follow the naming pattern:

```
<version>_<description>.sql
```

For example:
- `00001_create_todo_lists.sql`
- `00002_create_todo_items.sql`

Each migration file contains two sections:
- `-- +goose Up`: SQL statements to apply the migration
- `-- +goose Down`: SQL statements to rollback the migration

## Automatic Migrations

When you start the Mono service with SQL storage, migrations are applied automatically:

```bash
# For SQLite
MONO_STORAGE_TYPE=sqlite MONO_SQLITE_PATH=./data.db ./mono-server

# For PostgreSQL
MONO_STORAGE_TYPE=postgres MONO_POSTGRES_URL="postgres://user:pass@localhost:5432/mono" ./mono-server
```

## Manual Migration Management

### Creating a New Migration

Use the `make` command to create a new migration:

```bash
NAME=add_user_table make db-migrate-create
```

This creates a new file in `internal/storage/sql/migrations/` with the next version number.

### Running Migrations Up

To manually apply migrations:

```bash
# PostgreSQL
DB_DRIVER=postgres DB_URL="postgres://mono:mono_password@localhost:5432/mono_db" make db-migrate-up

# SQLite
DB_DRIVER=sqlite3 DB_URL="./data.db" make db-migrate-up
```

### Rolling Back Migrations

To rollback the last migration:

```bash
# PostgreSQL
DB_DRIVER=postgres DB_URL="postgres://mono:mono_password@localhost:5432/mono_db" make db-migrate-down

# SQLite
DB_DRIVER=sqlite3 DB_URL="./data.db" make db-migrate-down
```

## Database Schema

### todo_lists Table

Stores the top-level todo lists.

```sql
CREATE TABLE todo_lists (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### todo_items Table

Stores individual todo items belonging to lists.

```sql
CREATE TABLE todo_items (
    id TEXT PRIMARY KEY,
    list_id TEXT NOT NULL,
    title TEXT NOT NULL,
    completed INTEGER NOT NULL DEFAULT 0,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    due_time TIMESTAMP,
    tags TEXT,
    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE
);
```

**Notes:**
- `completed` is stored as INTEGER (0/1) for SQLite compatibility
- `tags` is stored as JSON text (JSON array serialized as string)
- `due_time` is nullable
- Foreign key cascade ensures items are deleted when their list is deleted

## Cross-Database Compatibility

The migrations are designed to work with both PostgreSQL and SQLite by:

1. Using TEXT for string types (compatible with both)
2. Using INTEGER for booleans (0/1)
3. Using TEXT for JSON data (both databases support JSON operations on TEXT)
4. Using TIMESTAMP for datetime fields
5. Avoiding database-specific features

### Trade-offs

- **JSON Storage**: We use TEXT instead of PostgreSQL's native JSONB for cross-compatibility. This means:
  - No native JSON indexing in PostgreSQL (but tags are small arrays)
  - Simple JSON arrays work well for our use case
  
- **Boolean Type**: Using INTEGER instead of native BOOLEAN:
  - More explicit storage (0 = false, 1 = true)
  - Works identically on both databases

- **Indexes**: Limited to simple B-tree indexes that work on both databases

## Troubleshooting

### Migration Failed

If a migration fails:

1. Check the error message in the logs
2. Verify your database connection string
3. Ensure the database exists and is accessible
4. Check if the migration SQL is compatible with your database

### Reset Database

To completely reset the database:

```bash
# SQLite - just delete the file
rm ./data.db

# PostgreSQL - drop and recreate
psql -U postgres -c "DROP DATABASE mono_db;"
psql -U postgres -c "CREATE DATABASE mono_db;"
```

Then restart the application to reapply migrations.

## Best Practices

1. **Never modify existing migrations** - Create a new migration instead
2. **Test migrations on both databases** - Ensure compatibility
3. **Keep migrations small** - One logical change per migration
4. **Always provide Down migrations** - Enable rollback capability
5. **Use transactions** - goose wraps migrations in transactions by default

## Development Workflow

1. Make schema changes by creating new migrations:
   ```bash
   NAME=add_priority_field make db-migrate-create
   ```

2. Edit the generated migration file with your SQL

3. Test with SQLite:
   ```bash
   MONO_STORAGE_TYPE=sqlite ./mono-server
   ```

4. Test with PostgreSQL:
   ```bash
   make db-up  # Start PostgreSQL in Docker
   MONO_STORAGE_TYPE=postgres MONO_POSTGRES_URL="postgres://mono:mono_password@localhost:5432/mono_db" ./mono-server
   ```

5. Update sqlc queries if needed:
   ```bash
   make gen-sqlc
   ```

6. Update repository code if needed

7. Run tests:
   ```bash
   make test-sql
   ```

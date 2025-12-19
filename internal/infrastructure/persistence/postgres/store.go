package postgres

import (
	"database/sql"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/storage/sql/sqlcgen"
)

// Store provides PostgreSQL implementation of all repository interfaces.
// It composes multiple repository implementations through interface satisfaction.
//
// This store implements:
// - application/auth.Repository (3 methods for API key operations)
// - application/todo.Repository (12 methods for todo and template operations)
// - application/worker.Repository (9 methods for job processing)
//
// The store uses sqlc-generated queries for type-safe SQL operations
// and converter functions to translate between database types and domain types.
type Store struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// Compile-time verification that Store implements all repository interfaces.
var (
	_ auth.Repository   = (*Store)(nil)
	_ todo.Repository   = (*Store)(nil)
	_ worker.Repository = (*Store)(nil)
)

// NewStore creates a new PostgreSQL store with the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		queries: sqlcgen.New(db),
	}
}

// DB returns the underlying database connection.
// This is useful for transaction management and migrations.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Queries returns the sqlc-generated queries for advanced use cases.
// Prefer using the repository interface methods when possible.
func (s *Store) Queries() *sqlcgen.Queries {
	return s.queries
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

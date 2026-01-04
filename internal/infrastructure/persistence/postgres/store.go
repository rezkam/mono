package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/infrastructure/persistence/postgres/sqlcgen"
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
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// Compile-time verification that Store implements all repository interfaces.
var (
	_ auth.Repository          = (*Store)(nil)
	_ todo.Repository          = (*Store)(nil)
	_ worker.Repository        = (*Store)(nil)
	_ todo.RecurringOperations = (*Store)(nil) // Verify Store implements RecurringOperations
)

// NewStore creates a new PostgreSQL store with the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}
}

// Pool returns the underlying connection pool.
// This is useful for transaction management and raw queries.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Queries returns the sqlc-generated queries for advanced use cases.
// Prefer using the repository interface methods when possible.
func (s *Store) Queries() *sqlcgen.Queries {
	return s.queries
}

// Close closes the database connection pool.
func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

// Atomic executes a callback function within a database transaction.
// All operations inside the callback succeed together or fail together.
// The callback receives a Repository instance that operates within the transaction.
// Commits the transaction if callback returns nil, rolls back if callback returns an error.
func (s *Store) Atomic(ctx context.Context, fn func(repo todo.Repository) error) (err error) {
	// Begin transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure transaction is closed
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				err = fmt.Errorf("transaction failed: %w (rollback error: %v)", err, rbErr)
			}
		} else {
			err = tx.Commit(ctx)
		}
	}()

	// Create a new store instance with the transaction
	txStore := &Store{
		pool:    s.pool,
		queries: s.queries.WithTx(tx),
	}

	// Execute callback with transactional repository
	err = fn(txStore)
	return
}

// AtomicRecurring executes a callback with recurring template operations in a transaction.
func (s *Store) AtomicRecurring(ctx context.Context, fn func(ops todo.RecurringOperations) error) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		// Handle panics by rolling back and re-panicking
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}

		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				err = fmt.Errorf("transaction failed: %w (rollback error: %v)", err, rbErr)
			}
		} else {
			err = tx.Commit(ctx)
		}
	}()

	// Create transactional store
	txStore := &Store{
		pool:    s.pool,
		queries: s.queries.WithTx(tx),
	}

	// txStore implements todo.Repository and worker.Repository (which provides the 4 methods),
	// so it satisfies todo.RecurringOperations
	err = fn(txStore)
	return
}

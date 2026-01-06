package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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

// finalizeTx handles transaction cleanup for normal error/success cases.
// Rolls back on error, commits on success.
// Note: Panics are handled separately in the defer blocks before finalizeTx is called.
func finalizeTx(ctx context.Context, tx pgx.Tx, err *error) {
	if *err != nil {
		slog.ErrorContext(ctx, "transaction failed, rolling back",
			"error", *err)
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			slog.ErrorContext(ctx, "rollback failed",
				"original_error", *err,
				"rollback_error", rbErr)
			*err = fmt.Errorf("transaction failed: %w (rollback error: %v)", *err, rbErr)
		}
	} else {
		*err = tx.Commit(ctx)
		if *err != nil {
			slog.ErrorContext(ctx, "transaction commit failed",
				"error", *err)
		}
	}
}

// executeInTransaction is a helper that executes a callback within a transaction with logging and panic recovery.
func (s *Store) executeInTransaction(ctx context.Context, operationName string, fn func(txStore *Store) error) (err error) {
	start := time.Now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to begin transaction",
			"operation", operationName,
			"error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			slog.ErrorContext(ctx, "transaction panic, rolling back",
				"operation", operationName,
				"panic", p)
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				slog.ErrorContext(ctx, "rollback after panic failed",
					"operation", operationName,
					"panic", p,
					"rollback_error", rbErr)
			}
			panic(p)
		}

		finalizeTx(ctx, tx, &err)
		if err == nil {
			slog.DebugContext(ctx, "transaction completed",
				"operation", operationName,
				"duration_ms", time.Since(start).Milliseconds())
		}
	}()

	txStore := &Store{
		pool:    s.pool,
		queries: s.queries.WithTx(tx),
	}

	err = fn(txStore)
	return
}

// Atomic executes a callback function within a database transaction.
// All operations inside the callback succeed together or fail together.
// The callback receives a Repository instance that operates within the transaction.
// Commits the transaction if callback returns nil, rolls back if callback returns an error.
func (s *Store) Atomic(ctx context.Context, fn func(repo todo.Repository) error) error {
	return s.executeInTransaction(ctx, "atomic", func(txStore *Store) error {
		return fn(txStore)
	})
}

// AtomicRecurring executes a callback with recurring template operations in a transaction.
func (s *Store) AtomicRecurring(ctx context.Context, fn func(ops todo.RecurringOperations) error) error {
	return s.executeInTransaction(ctx, "atomic_recurring", func(txStore *Store) error {
		return fn(txStore)
	})
}

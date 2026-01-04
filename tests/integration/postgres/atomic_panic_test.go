package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtomic_RollsBackOnPanic verifies that when a callback panics inside Atomic(),
// the transaction is rolled back and the panic is re-raised.
//
// This is a critical safety guarantee: panics must not leave partial data committed.
func TestAtomic_RollsBackOnPanic(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Prepare a list to be created inside the transaction
	listID := uuid.Must(uuid.NewV7()).String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Test List",
		CreatedAt: time.Now().UTC(),
	}

	// Execute transaction with panic - should be caught and rolled back
	assert.Panics(t, func() {
		_ = store.Atomic(ctx, func(repo todo.Repository) error {
			// Create list inside transaction
			_, err := repo.CreateList(ctx, list)
			if err != nil {
				return err
			}
			// Panic after successful insert
			panic("simulated panic")
		})
	}, "Atomic() should re-raise the panic")

	// Verify list was NOT committed (rolled back due to panic)
	_, err := store.FindListByID(ctx, listID)
	assert.ErrorIs(t, err, domain.ErrListNotFound, "List should NOT exist after panic rollback")
}

// TestAtomic_RollsBackOnError verifies that when a callback returns an error,
// the transaction is rolled back (baseline behavior that should already work).
func TestAtomic_RollsBackOnError(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	listID := uuid.Must(uuid.NewV7()).String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Test List",
		CreatedAt: time.Now().UTC(),
	}

	testErr := errors.New("simulated error")

	// Execute transaction with error return
	err := store.Atomic(ctx, func(repo todo.Repository) error {
		_, err := repo.CreateList(ctx, list)
		if err != nil {
			return err
		}
		return testErr // Return error after insert
	})

	require.ErrorIs(t, err, testErr, "Should return the error from callback")

	// Verify list was NOT committed (rolled back due to error)
	_, err = store.FindListByID(ctx, listID)
	assert.ErrorIs(t, err, domain.ErrListNotFound, "List should NOT exist after error rollback")
}

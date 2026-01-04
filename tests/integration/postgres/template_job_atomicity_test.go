package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAtomicityTest initializes test environment for atomicity testing
func setupAtomicityTest(t *testing.T) (*postgres.Store, func()) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	// Clean up tables before test
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_template_exceptions, recurring_generation_jobs, dead_letter_jobs, api_keys CASCADE")
	require.NoError(t, err)

	cleanup := func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_template_exceptions, recurring_generation_jobs, dead_letter_jobs, api_keys CASCADE")
		store.Close()
	}

	return store, cleanup
}

// createListForAtomicityTest creates a test list and returns its ID
func createListForAtomicityTest(t *testing.T, store *postgres.Store, title string) string {
	t.Helper()
	listID := uuid.Must(uuid.NewV7()).String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     title,
		CreatedAt: time.Now().UTC(),
	}
	_, err := store.CreateList(context.Background(), list)
	require.NoError(t, err)
	return listID
}

// TestTemplateJobAtomicity_CommitsAtomically verifies that template creation
// and job creation within a transaction are both committed together.
// This tests the infrastructure-level atomicity guarantee.
func TestTemplateJobAtomicity_CommitsAtomically(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list first
	listID := createListForAtomicityTest(t, store, "Test List")

	// Prepare template and job
	templateID := uuid.Must(uuid.NewV7()).String()
	jobID := uuid.Must(uuid.NewV7()).String()

	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}

	job := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    templateID,
		GenerateFrom:  time.Now().UTC(),
		GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
		ScheduledFor:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Execute transaction - both should commit together
	err := store.Atomic(ctx, func(tx todo.Repository) error {
		_, err := tx.CreateRecurringTemplate(ctx, template)
		if err != nil {
			return err
		}
		return tx.CreateGenerationJob(ctx, job)
	})
	require.NoError(t, err)

	// Verify both were committed
	foundTemplate, err := store.FindRecurringTemplateByID(ctx, templateID)
	require.NoError(t, err)
	assert.Equal(t, templateID, foundTemplate.ID)

	var jobStatus string
	err = store.Pool().QueryRow(ctx,
		"SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&jobStatus)
	require.NoError(t, err)
	assert.Equal(t, "pending", jobStatus)
}

// TestTemplateJobAtomicity_RollsBackOnError verifies that when transaction
// callback returns an error, neither template nor job is committed.
func TestTemplateJobAtomicity_RollsBackOnError(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list first
	listID := createListForAtomicityTest(t, store, "Test List")

	// Prepare template and job
	templateID := uuid.Must(uuid.NewV7()).String()
	jobID := uuid.Must(uuid.NewV7()).String()

	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}

	job := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    templateID,
		GenerateFrom:  time.Now().UTC(),
		GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
		ScheduledFor:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Execute transaction - return error after both inserts
	testErr := errors.New("simulated failure")
	err := store.Atomic(ctx, func(tx todo.Repository) error {
		_, err := tx.CreateRecurringTemplate(ctx, template)
		if err != nil {
			return err
		}
		if err := tx.CreateGenerationJob(ctx, job); err != nil {
			return err
		}
		return testErr // Simulate failure after inserts
	})
	require.ErrorIs(t, err, testErr)

	// Verify NEITHER was committed (rolled back)
	_, err = store.FindRecurringTemplateByID(ctx, templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template should NOT exist after rollback")

	var count int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Job should NOT exist after rollback")
}

// TestTemplateJobAtomicity_RollsBackOnPanic verifies that when transaction
// callback panics, neither template nor job is committed.
func TestTemplateJobAtomicity_RollsBackOnPanic(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list first
	listID := createListForAtomicityTest(t, store, "Test List")

	// Prepare template and job
	templateID := uuid.Must(uuid.NewV7()).String()
	jobID := uuid.Must(uuid.NewV7()).String()

	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}

	job := &domain.GenerationJob{
		ID:            jobID,
		TemplateID:    templateID,
		GenerateFrom:  time.Now().UTC(),
		GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
		ScheduledFor:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Execute transaction with panic - should be caught and rolled back
	assert.Panics(t, func() {
		_ = store.Atomic(ctx, func(tx todo.Repository) error {
			_, err := tx.CreateRecurringTemplate(ctx, template)
			if err != nil {
				return err
			}
			if err := tx.CreateGenerationJob(ctx, job); err != nil {
				return err
			}
			panic("simulated panic")
		})
	})

	// Verify NEITHER was committed (rolled back due to panic)
	_, err := store.FindRecurringTemplateByID(ctx, templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template should NOT exist after panic rollback")

	var count int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Job should NOT exist after panic rollback")
}

// TestTemplateJobAtomicity_PartialInsertRollback verifies that if job insertion
// fails, the template is also rolled back.
func TestTemplateJobAtomicity_PartialInsertRollback(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list first
	listID := createListForAtomicityTest(t, store, "Test List")

	// Prepare template with valid ID
	templateID := uuid.Must(uuid.NewV7()).String()

	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Daily Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}

	// Job with invalid template ID to cause failure
	invalidJob := &domain.GenerationJob{
		ID:            uuid.Must(uuid.NewV7()).String(),
		TemplateID:    "invalid-uuid", // Invalid ID will cause FK constraint failure
		GenerateFrom:  time.Now().UTC(),
		GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
		ScheduledFor:  time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}

	// Execute transaction - job insert should fail, rolling back template
	err := store.Atomic(ctx, func(tx todo.Repository) error {
		_, err := tx.CreateRecurringTemplate(ctx, template)
		if err != nil {
			return err
		}
		return tx.CreateGenerationJob(ctx, invalidJob)
	})
	require.Error(t, err, "Should fail due to invalid job template ID")

	// Verify template was also rolled back
	_, err = store.FindRecurringTemplateByID(ctx, templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template should NOT exist after job insert failure")
}

// TestTemplateJobAtomicity_NestedOperations verifies multiple operations
// within the same transaction are all atomic.
func TestTemplateJobAtomicity_NestedOperations(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a list first
	listID := createListForAtomicityTest(t, store, "Test List")

	// Prepare multiple templates and jobs
	template1ID := uuid.Must(uuid.NewV7()).String()
	template2ID := uuid.Must(uuid.NewV7()).String()
	job1ID := uuid.Must(uuid.NewV7()).String()
	job2ID := uuid.Must(uuid.NewV7()).String()

	// Execute transaction with multiple operations, then fail
	testErr := errors.New("simulated failure after multiple inserts")
	err := store.Atomic(ctx, func(tx todo.Repository) error {
		// Create first template and job
		_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                template1ID,
			ListID:            listID,
			Title:             "Template 1",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		if err := tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            job1ID,
			TemplateID:    template1ID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		}); err != nil {
			return err
		}

		// Create second template and job
		_, err = tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                template2ID,
			ListID:            listID,
			Title:             "Template 2",
			RecurrencePattern: domain.RecurrenceWeekly,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		if err := tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            job2ID,
			TemplateID:    template2ID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		}); err != nil {
			return err
		}

		return testErr // Fail after all inserts
	})
	require.ErrorIs(t, err, testErr)

	// Verify ALL operations were rolled back
	_, err = store.FindRecurringTemplateByID(ctx, template1ID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template 1 should NOT exist")

	_, err = store.FindRecurringTemplateByID(ctx, template2ID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template 2 should NOT exist")

	var count int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM recurring_generation_jobs WHERE id IN ($1, $2)", job1ID, job2ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "Neither job should exist after rollback")
}

// TestTemplateJobAtomicity_FirstOperationFails verifies that when template
// creation fails, the transaction returns error without attempting job creation.
func TestTemplateJobAtomicity_FirstOperationFails(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()

	// Use non-existent list ID to cause template creation to fail
	nonExistentListID := uuid.Must(uuid.NewV7()).String()
	templateID := uuid.Must(uuid.NewV7()).String()
	jobID := uuid.Must(uuid.NewV7()).String()

	jobCreationAttempted := false

	err := store.Atomic(ctx, func(tx todo.Repository) error {
		// Template with invalid list ID - should fail
		_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                templateID,
			ListID:            nonExistentListID, // FK violation
			Title:             "Daily Task",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err // Should return here without attempting job creation
		}

		// This should never execute
		jobCreationAttempted = true
		return tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            jobID,
			TemplateID:    templateID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		})
	})

	require.Error(t, err, "Transaction should fail due to FK constraint")
	assert.False(t, jobCreationAttempted, "Job creation should not be attempted after template fails")

	// Verify neither exists
	_, err = store.FindRecurringTemplateByID(ctx, templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound)

	var count int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestTemplateJobAtomicity_DuplicateTemplateID verifies unique constraint
// violation rolls back cleanly.
func TestTemplateJobAtomicity_DuplicateTemplateID(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()
	listID := createListForAtomicityTest(t, store, "Test List")

	// Create an existing template outside transaction
	existingTemplateID := uuid.Must(uuid.NewV7()).String()
	_, err := store.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ID:                existingTemplateID,
		ListID:            listID,
		Title:             "Existing Template",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	})
	require.NoError(t, err)

	newTemplateID := uuid.Must(uuid.NewV7()).String()
	jobID := uuid.Must(uuid.NewV7()).String()

	// Try to create template with duplicate ID in transaction
	err = store.Atomic(ctx, func(tx todo.Repository) error {
		// First create a new valid template
		_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                newTemplateID,
			ListID:            listID,
			Title:             "New Template",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		// Then try to create duplicate - should fail
		_, err = tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                existingTemplateID, // Duplicate!
			ListID:            listID,
			Title:             "Duplicate Template",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		return tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            jobID,
			TemplateID:    newTemplateID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		})
	})
	require.Error(t, err, "Transaction should fail due to duplicate template ID")

	// Verify the new template was rolled back (only existing remains)
	_, err = store.FindRecurringTemplateByID(ctx, newTemplateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "New template should be rolled back")

	// Existing template should still exist
	_, err = store.FindRecurringTemplateByID(ctx, existingTemplateID)
	assert.NoError(t, err, "Existing template should still exist")
}

// TestTemplateJobAtomicity_ContextCancellation verifies that cancelled context
// aborts transaction and rolls back.
func TestTemplateJobAtomicity_ContextCancellation(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	listID := createListForAtomicityTest(t, store, "Test List")
	templateID := uuid.Must(uuid.NewV7()).String()

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	err := store.Atomic(ctx, func(tx todo.Repository) error {
		_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                templateID,
			ListID:            listID,
			Title:             "Daily Task",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		// Cancel context before job creation
		cancel()

		// Try to create job with cancelled context - should fail
		return tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            uuid.Must(uuid.NewV7()).String(),
			TemplateID:    templateID,
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		})
	})

	require.Error(t, err, "Transaction should fail due to context cancellation")
	assert.ErrorIs(t, err, context.Canceled, "Error should be context.Canceled")

	// Verify template was rolled back
	_, err = store.FindRecurringTemplateByID(context.Background(), templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound, "Template should be rolled back after context cancellation")
}

// TestTemplateJobAtomicity_TransactionIsolation verifies that uncommitted data
// is not visible to other connections (read committed isolation).
func TestTemplateJobAtomicity_TransactionIsolation(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()
	listID := createListForAtomicityTest(t, store, "Test List")
	templateID := uuid.Must(uuid.NewV7()).String()

	// Channel to coordinate between goroutines
	templateCreated := make(chan struct{})
	checkComplete := make(chan struct{})

	var isolationErr error

	// Start transaction that creates template but doesn't commit yet
	go func() {
		_ = store.Atomic(ctx, func(tx todo.Repository) error {
			_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
				ID:                templateID,
				ListID:            listID,
				Title:             "Daily Task",
				RecurrencePattern: domain.RecurrenceDaily,
				RecurrenceConfig:  map[string]any{},
				IsActive:          true,
				CreatedAt:         time.Now().UTC(),
				UpdatedAt:         time.Now().UTC(),
				GeneratedThrough:  time.Now().UTC(),
			})
			if err != nil {
				return err
			}

			// Signal that template was created (but not committed)
			close(templateCreated)

			// Wait for isolation check to complete
			<-checkComplete

			// Return error to rollback
			return errors.New("intentional rollback")
		})
	}()

	// Wait for template to be created in the other transaction
	<-templateCreated

	// Try to read template from a different connection - should NOT be visible
	_, isolationErr = store.FindRecurringTemplateByID(ctx, templateID)

	// Signal that check is complete
	close(checkComplete)

	// Template should NOT be visible (uncommitted)
	assert.ErrorIs(t, isolationErr, domain.ErrTemplateNotFound,
		"Uncommitted template should NOT be visible to other connections")
}

// TestTemplateJobAtomicity_JobReferencesNonExistentTemplate verifies that
// creating a job referencing a template that doesn't exist (valid UUID but not in DB)
// fails with FK constraint and rolls back any prior operations.
func TestTemplateJobAtomicity_JobReferencesNonExistentTemplate(t *testing.T) {
	store, cleanup := setupAtomicityTest(t)
	defer cleanup()

	ctx := context.Background()
	listID := createListForAtomicityTest(t, store, "Test List")

	templateID := uuid.Must(uuid.NewV7()).String()
	nonExistentTemplateID := uuid.Must(uuid.NewV7()).String() // Valid UUID but not in DB
	jobID := uuid.Must(uuid.NewV7()).String()

	err := store.Atomic(ctx, func(tx todo.Repository) error {
		// Create valid template first
		_, err := tx.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
			ID:                templateID,
			ListID:            listID,
			Title:             "Daily Task",
			RecurrencePattern: domain.RecurrenceDaily,
			RecurrenceConfig:  map[string]any{},
			IsActive:          true,
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
			GeneratedThrough:  time.Now().UTC(),
		})
		if err != nil {
			return err
		}

		// Try to create job referencing non-existent template
		return tx.CreateGenerationJob(ctx, &domain.GenerationJob{
			ID:            jobID,
			TemplateID:    nonExistentTemplateID, // Valid UUID but doesn't exist
			GenerateFrom:  time.Now().UTC(),
			GenerateUntil: time.Now().UTC().AddDate(0, 0, 7),
			ScheduledFor:  time.Now().UTC(),
			CreatedAt:     time.Now().UTC(),
		})
	})

	require.Error(t, err, "Transaction should fail due to FK constraint")

	// Verify template was rolled back
	_, err = store.FindRecurringTemplateByID(ctx, templateID)
	assert.ErrorIs(t, err, domain.ErrTemplateNotFound,
		"Template should be rolled back when job FK fails")

	// Verify job doesn't exist
	var count int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

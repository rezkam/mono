package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkJobAsCancelled_DoesNotViolateNotNullConstraint verifies that
// cancelling a running job does not violate the available_at NOT NULL constraint.
//
// BUG: The SQL query sets available_at = NULL, but the schema requires NOT NULL.
// This test will fail with a constraint violation until the bug is fixed.
func TestMarkJobAsCancelled_DoesNotViolateNotNullConstraint(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create test list and template
	listID := uuid.Must(uuid.NewV7()).String()
	store.CreateList(ctx, &domain.TodoList{ID: listID, Title: "Test List"})

	templateID := uuid.Must(uuid.NewV7()).String()
	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Test Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	}
	store.CreateRecurringTemplate(ctx, template)

	// Create a pending job
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))
	require.NoError(t, err)

	// Claim the job (marks it as running)
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, jobID, job.ID)

	// Verify job is running
	var statusBefore string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).
		Scan(&statusBefore)
	require.NoError(t, err)
	assert.Equal(t, "running", statusBefore)

	// Request cancellation (sets status to 'cancelling')
	rowsAffected, err := coordinator.RequestCancellation(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	// Verify status is now 'cancelling'
	var statusCancelling string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).
		Scan(&statusCancelling)
	require.NoError(t, err)
	assert.Equal(t, "cancelling", statusCancelling)

	// Mark job as cancelled (this should NOT violate NOT NULL constraint)
	cancelRows, err := coordinator.MarkJobAsCancelled(ctx, jobID, cfg.WorkerID)
	require.NoError(t, err, "MarkJobAsCancelled should not violate NOT NULL constraint on available_at")
	assert.Equal(t, int64(1), cancelRows, "Should update exactly one row")

	// Verify job is now cancelled
	var statusAfter string
	var availableAt *time.Time
	err = store.Pool().QueryRow(ctx,
		"SELECT status, available_at FROM recurring_generation_jobs WHERE id = $1", jobID).
		Scan(&statusAfter, &availableAt)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", statusAfter)

	// CRITICAL: available_at must NOT be NULL (schema constraint)
	assert.NotNil(t, availableAt, "available_at must not be NULL (violates NOT NULL constraint)")
}

// TestMarkJobAsCancelled_OnlyAffectsOwnedJobs verifies ownership check.
func TestMarkJobAsCancelled_OnlyAffectsOwnedJobs(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create test data
	listID := uuid.Must(uuid.NewV7()).String()
	store.CreateList(ctx, &domain.TodoList{ID: listID, Title: "Test List"})

	templateID := uuid.Must(uuid.NewV7()).String()
	store.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Test Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	})

	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))
	require.NoError(t, err)

	// Claim job with worker-1
	job, err := coordinator.ClaimNextJob(ctx, "worker-1", 1*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Request cancellation
	_, err = coordinator.RequestCancellation(ctx, jobID)
	require.NoError(t, err)

	// Try to mark as cancelled with wrong worker-2 (should fail ownership check)
	cancelRows, err := coordinator.MarkJobAsCancelled(ctx, jobID, "worker-2")
	require.NoError(t, err)
	assert.Equal(t, int64(0), cancelRows, "Should not cancel job owned by different worker")

	// Verify job is still in 'cancelling' state
	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).
		Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "cancelling", status, "Job should remain in cancelling state")

	// Mark as cancelled with correct worker (should succeed)
	cancelRows, err = coordinator.MarkJobAsCancelled(ctx, jobID, "worker-1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), cancelRows, "Should cancel job when worker matches")

	// Verify final state
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).
		Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", status)
}

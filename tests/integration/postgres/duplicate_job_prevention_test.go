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

// TestWorker_DuplicateJobPrevention verifies that calling RunScheduleOnce multiple
// times does not create duplicate jobs for templates that already have pending/running jobs.
//
// RunScheduleOnce checks HasPendingOrRunningJob before creating a new job.
// If a pending or running job already exists for the template, no new job is created.
func TestWorker_DuplicateJobPrevention(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Clean up tables before test
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	// Cleanup after test
	defer func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	}()

	w := worker.New(store)

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Duplicate Prevention Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template that needs generation
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Daily Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	t.Run("first_schedule_creates_one_job", func(t *testing.T) {
		// First schedule - should create exactly one job
		err := w.RunScheduleOnce(ctx)
		require.NoError(t, err)

		var jobCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1
		`, templateID).Scan(&jobCount)
		require.NoError(t, err)

		assert.Equal(t, 1, jobCount, "First RunScheduleOnce should create exactly one job")
	})

	t.Run("second_schedule_does_not_create_duplicate", func(t *testing.T) {
		// Second schedule - should NOT create another job because one is already pending
		err := w.RunScheduleOnce(ctx)
		require.NoError(t, err)

		var jobCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1
		`, templateID).Scan(&jobCount)
		require.NoError(t, err)

		assert.Equal(t, 1, jobCount, "Second RunScheduleOnce should NOT create duplicate job")
	})

	t.Run("third_schedule_still_no_duplicate", func(t *testing.T) {
		// Third schedule - still should not create another job
		err := w.RunScheduleOnce(ctx)
		require.NoError(t, err)

		var jobCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1
		`, templateID).Scan(&jobCount)
		require.NoError(t, err)

		assert.Equal(t, 1, jobCount, "Third RunScheduleOnce should NOT create duplicate job")
	})

	t.Run("after_job_completion_can_create_new_job_if_needed", func(t *testing.T) {
		// Process the pending job using GenerationWorker
		genWorker := setupGenerationWorkerTest(t, store)
		err := genWorker.RunProcessOnce(ctx)
		require.NoError(t, err)

		// Verify job is completed
		var completedCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1 AND status = 'completed'
		`, templateID).Scan(&completedCount)
		require.NoError(t, err)
		assert.Equal(t, 1, completedCount, "Job should be completed")

		// Schedule again - template's generated_through is now updated
		// So a new job may or may not be needed depending on the window
		err = w.RunScheduleOnce(ctx)
		require.NoError(t, err)

		// At this point, no new pending job should be created because
		// generated_through was updated to cover the window
		var pendingCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1 AND status = 'pending'
		`, templateID).Scan(&pendingCount)
		require.NoError(t, err)
		assert.Equal(t, 0, pendingCount, "No new pending job needed after window is covered")
	})
}

// TestWorker_DuplicateJobPrevention_RunningJob verifies that templates with
// running jobs also don't get duplicate jobs created.
func TestWorker_DuplicateJobPrevention_RunningJob(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Clean up tables
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	defer func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	}()

	w := worker.New(store)

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Running Job Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Weekly Task",
		RecurrencePattern:     domain.RecurrenceWeekly,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create a job and claim it (putting it in running state)
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	// Claim the job using coordinator (changes status from pending to running)
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job, "Should have claimed a job")

	// Verify job is running
	var runningCount int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1 AND status = 'running'
	`, templateID).Scan(&runningCount)
	require.NoError(t, err)
	assert.Equal(t, 1, runningCount, "Job should be in running state")

	// Try to schedule again - should NOT create another job
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var totalJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1
	`, templateID).Scan(&totalJobs)
	require.NoError(t, err)
	assert.Equal(t, 1, totalJobs, "Should NOT create duplicate job while one is running")
}

// TestWorker_MultipleTemplates_IndependentDuplicatePrevention verifies that
// duplicate prevention works independently for each template.
func TestWorker_MultipleTemplates_IndependentDuplicatePrevention(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	// Clean up tables
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	defer func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	}()

	w := worker.New(store)

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Multi Template Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 3 templates
	templateIDs := make([]string, 3)
	for i := range 3 {
		templateUUID, err := uuid.NewV7()
		require.NoError(t, err)
		templateIDs[i] = templateUUID.String()

		template := &domain.RecurringTemplate{
			ID:                    templateIDs[i],
			ListID:                listID,
			Title:                 "Task " + string(rune('A'+i)),
			RecurrencePattern:     domain.RecurrenceDaily,
			RecurrenceConfig:      map[string]any{"interval": float64(1)},
			SyncHorizonDays:       14,
			GenerationHorizonDays: 365,
			IsActive:              true,
			CreatedAt:             time.Now().UTC(),
			UpdatedAt:             time.Now().UTC(),
		}
		_, err = store.CreateRecurringTemplate(ctx, template)
		require.NoError(t, err)
	}

	// First schedule - should create one job per template
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	for i, templateID := range templateIDs {
		var jobCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1
		`, templateID).Scan(&jobCount)
		require.NoError(t, err)
		assert.Equal(t, 1, jobCount, "Template %d should have exactly 1 job", i)
	}

	// Second schedule - no new jobs for any template
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var totalJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
	`).Scan(&totalJobs)
	require.NoError(t, err)
	assert.Equal(t, 3, totalJobs, "Should still have exactly 3 jobs total (no duplicates)")

	// Process one job using GenerationWorker
	genWorker := setupGenerationWorkerTest(t, store)
	err = genWorker.RunProcessOnce(ctx)
	require.NoError(t, err)

	// Third schedule - still no duplicates for the 2 pending templates
	// (the completed one already has its window covered)
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var pendingJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE status = 'pending'
	`).Scan(&pendingJobs)
	require.NoError(t, err)
	assert.Equal(t, 2, pendingJobs, "Should still have exactly 2 pending jobs")
}

// TestStore_HasPendingOrRunningJob_ScheduledStatus verifies that HasPendingOrRunningJob
// detects jobs in 'scheduled' status, not just 'pending' and 'running'.
func TestStore_HasPendingOrRunningJob_ScheduledStatus(t *testing.T) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)
	defer store.Close()

	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	defer func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	}()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Scheduled Status Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Future Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create a job with 'scheduled' status
	jobUUID, err := uuid.NewV7()
	require.NoError(t, err)
	jobID := jobUUID.String()
	futureTime := time.Now().UTC().Add(24 * time.Hour)

	_, err = store.Pool().Exec(ctx, `
		INSERT INTO recurring_generation_jobs (
			id, template_id, generate_from, generate_until,
			scheduled_for, status, retry_count, created_at
		) VALUES ($1, $2, $3, $4, $5, 'scheduled', 0, NOW())
	`, jobID, templateID, time.Now().UTC(), futureTime, futureTime)
	require.NoError(t, err)

	// HasPendingOrRunningJob SHOULD detect scheduled jobs
	hasJob, err := store.HasPendingOrRunningJob(ctx, templateID)
	require.NoError(t, err)
	assert.True(t, hasJob, "HasPendingOrRunningJob should return true for 'scheduled' jobs")
}

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
		ID:         listID,
		Title:      "Duplicate Prevention Test",
		Items:      []domain.TodoItem{},
		CreateTime: time.Now(),
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template that needs generation
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                   templateID,
		ListID:               listID,
		Title:                "Daily Task",
		RecurrencePattern:    domain.RecurrenceDaily,
		RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
		GenerationWindowDays: 30,
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	err = store.CreateRecurringTemplate(ctx, template)
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
		// Process the pending job
		processed, err := w.RunProcessOnce(ctx)
		require.NoError(t, err)
		assert.True(t, processed, "Should process the job")

		// Verify job is completed
		var completedCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1 AND status = 'COMPLETED'
		`, templateID).Scan(&completedCount)
		require.NoError(t, err)
		assert.Equal(t, 1, completedCount, "Job should be completed")

		// Schedule again - template's last_generated_until is now updated
		// So a new job may or may not be needed depending on the window
		err = w.RunScheduleOnce(ctx)
		require.NoError(t, err)

		// At this point, no new pending job should be created because
		// last_generated_until was updated to cover the window
		var pendingCount int
		err = store.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM recurring_generation_jobs
			WHERE template_id = $1 AND status = 'PENDING'
		`, templateID).Scan(&pendingCount)
		require.NoError(t, err)
		assert.Equal(t, 0, pendingCount, "No new pending job needed after window is covered")
	})
}

// TestWorker_DuplicateJobPrevention_RunningJob verifies that templates with
// RUNNING jobs also don't get duplicate jobs created.
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
		ID:         listID,
		Title:      "Running Job Test",
		Items:      []domain.TodoItem{},
		CreateTime: time.Now(),
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                   templateID,
		ListID:               listID,
		Title:                "Weekly Task",
		RecurrencePattern:    domain.RecurrenceWeekly,
		RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
		GenerationWindowDays: 60,
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create a job and claim it (putting it in RUNNING state)
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	// Claim the job (changes status from PENDING to RUNNING)
	jobID, err := store.ClaimNextGenerationJob(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, jobID, "Should have claimed a job")

	// Verify job is RUNNING
	var runningCount int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1 AND status = 'RUNNING'
	`, templateID).Scan(&runningCount)
	require.NoError(t, err)
	assert.Equal(t, 1, runningCount, "Job should be in RUNNING state")

	// Try to schedule again - should NOT create another job
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var totalJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1
	`, templateID).Scan(&totalJobs)
	require.NoError(t, err)
	assert.Equal(t, 1, totalJobs, "Should NOT create duplicate job while one is RUNNING")
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
		ID:         listID,
		Title:      "Multi Template Test",
		Items:      []domain.TodoItem{},
		CreateTime: time.Now(),
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 3 templates
	templateIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		templateUUID, err := uuid.NewV7()
		require.NoError(t, err)
		templateIDs[i] = templateUUID.String()

		template := &domain.RecurringTemplate{
			ID:                   templateIDs[i],
			ListID:               listID,
			Title:                "Task " + string(rune('A'+i)),
			RecurrencePattern:    domain.RecurrenceDaily,
			RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
			GenerationWindowDays: 30,
			IsActive:             true,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err = store.CreateRecurringTemplate(ctx, template)
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

	// Process one job
	processed, err := w.RunProcessOnce(ctx)
	require.NoError(t, err)
	assert.True(t, processed)

	// Third schedule - still no duplicates for the 2 pending templates
	// (the completed one already has its window covered)
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var pendingJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE status = 'PENDING'
	`).Scan(&pendingJobs)
	require.NoError(t, err)
	assert.Equal(t, 2, pendingJobs, "Should still have exactly 2 pending jobs")
}

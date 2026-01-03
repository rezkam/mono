package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/application/worker"
	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWorkerTest initializes a test environment with storage and workers.
// Returns the store, a scheduler Worker, a GenerationWorker for processing, and cleanup.
func setupWorkerTest(t *testing.T) (*postgres.Store, *worker.Worker, func()) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	// Clean up tables before test
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, dead_letter_jobs, api_keys CASCADE")
	require.NoError(t, err)

	w := worker.New(store)

	cleanup := func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, dead_letter_jobs, api_keys CASCADE")
		store.Close()
	}

	return store, w, cleanup
}

// setupGenerationWorkerTest returns a GenerationWorker for processing tests.
func setupGenerationWorkerTest(t *testing.T, store *postgres.Store) *worker.GenerationWorker {
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	generator := recurring.NewDomainGenerator()
	cfg := worker.DefaultWorkerConfig("test-worker")

	return worker.NewGenerationWorker(coordinator, store, generator, cfg)
}

// TestWorker_CompleteFlow tests the end-to-end flow with multiple recurrence patterns.
func TestWorker_CompleteFlow(t *testing.T) {
	store, w, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test cases with different patterns
	testCases := []struct {
		name             string
		pattern          domain.RecurrencePattern
		config           map[string]any
		generationWindow int
		expectedMinTasks int // Minimum tasks expected
		expectedMaxTasks int // Maximum tasks expected
	}{
		{
			name:             "Daily",
			pattern:          domain.RecurrenceDaily,
			config:           map[string]any{"interval": float64(1)},
			generationWindow: 30,
			expectedMinTasks: 30,
			expectedMaxTasks: 31, // Account for edge cases
		},
		{
			name:             "Weekly",
			pattern:          domain.RecurrenceWeekly,
			config:           map[string]any{"interval": float64(1)},
			generationWindow: 90,
			expectedMinTasks: 12,
			expectedMaxTasks: 14,
		},
		{
			name:             "Monthly",
			pattern:          domain.RecurrenceMonthly,
			config:           map[string]any{"interval": float64(1)},
			generationWindow: 365,
			expectedMinTasks: 12,
			expectedMaxTasks: 15, // 12-13 months + potential boundary edge cases
		},
		{
			name:             "Weekdays",
			pattern:          domain.RecurrenceWeekdays,
			config:           map[string]any{},
			generationWindow: 30,
			expectedMinTasks: 21,
			expectedMaxTasks: 23,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up any leftover templates/jobs from previous subtests
			_, err := store.Pool().Exec(ctx, "DELETE FROM recurring_generation_jobs")
			require.NoError(t, err)
			_, err = store.Pool().Exec(ctx, "DELETE FROM recurring_task_templates")
			require.NoError(t, err)

			// Create fresh list for this subtest
			listUUID, err := uuid.NewV7()
			require.NoError(t, err, "failed to generate list UUID")
			listID := listUUID.String()
			list := &domain.TodoList{
				ID:        listID,
				Title:     fmt.Sprintf("Test List - %s", tc.name),
				CreatedAt: time.Now().UTC(),
			}
			_, err = store.CreateList(ctx, list)
			require.NoError(t, err)

			// Create template
			templateUUID, err := uuid.NewV7()
			require.NoError(t, err, "failed to generate template UUID")
			templateID := templateUUID.String()
			template := &domain.RecurringTemplate{
				ID:                    templateID,
				ListID:                listID,
				Title:                 fmt.Sprintf("Test Task - %s", tc.name),
				RecurrencePattern:     tc.pattern,
				RecurrenceConfig:      tc.config,
				SyncHorizonDays:       14,
				GenerationHorizonDays: tc.generationWindow,
				IsActive:              true,
				CreatedAt:             time.Now().UTC(),
				UpdatedAt:             time.Now().UTC(),
			}
			_, err = store.CreateRecurringTemplate(ctx, template)
			require.NoError(t, err)

			// Step 1: Schedule job
			err = w.RunScheduleOnce(ctx)
			require.NoError(t, err)

			// Verify job was created
			var jobCount int
			err = store.Pool().QueryRow(ctx, `
				SELECT COUNT(*) FROM recurring_generation_jobs
				WHERE template_id = $1 AND status = 'pending'
			`, template.ID).Scan(&jobCount)
			require.NoError(t, err)
			assert.Equal(t, 1, jobCount, "Should have created one pending job")

			// Step 2: Process job using GenerationWorker
			genWorker := setupGenerationWorkerTest(t, store)
			err = genWorker.RunProcessOnce(ctx)
			require.NoError(t, err)

			// Verify job completed
			var completedCount int
			err = store.Pool().QueryRow(ctx, `
				SELECT COUNT(*) FROM recurring_generation_jobs
				WHERE template_id = $1 AND status = 'completed'
			`, template.ID).Scan(&completedCount)
			require.NoError(t, err)
			assert.Equal(t, 1, completedCount, "Job should be completed")

			// Verify tasks were created
			itemFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
			require.NoError(t, err)
			itemsResult, err := store.FindItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: itemFilter, Limit: 1000}, nil)
			require.NoError(t, err)

			taskCount := len(itemsResult.Items)
			assert.GreaterOrEqual(t, taskCount, tc.expectedMinTasks,
				"Should have at least %d tasks", tc.expectedMinTasks)
			assert.LessOrEqual(t, taskCount, tc.expectedMaxTasks,
				"Should have at most %d tasks", tc.expectedMaxTasks)

			// Verify tasks span into the future
			if taskCount > 0 {
				firstTask := itemsResult.Items[0]
				lastTask := itemsResult.Items[taskCount-1]

				if lastTask.DueAt != nil && firstTask.DueAt != nil {
					assert.True(t, lastTask.DueAt.After(*firstTask.DueAt),
						"Tasks should span across time")

					// For longer windows, verify substantial time span
					if tc.generationWindow >= 90 {
						daysBetween := lastTask.DueAt.Sub(*firstTask.DueAt).Hours() / 24
						assert.Greater(t, daysBetween, 60.0,
							"Tasks should span at least 60 days for %s pattern", tc.pattern)
					}
				}
			}

			// Verify template was updated
		})
	}
}

// TestWorker_MultipleWorkers_JobDistribution tests multiple workers processing jobs without conflicts.
func TestWorker_MultipleWorkers_JobDistribution(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate list UUID")
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Distribution Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 10 templates
	numTemplates := 10
	templateIDs := make([]string, numTemplates)
	for i := range numTemplates {
		templateUUID, err := uuid.NewV7()
		require.NoError(t, err, "failed to generate template UUID")
		templateID := templateUUID.String()
		templateIDs[i] = templateID

		template := &domain.RecurringTemplate{
			ID:                    templateID,
			ListID:                listID,
			Title:                 fmt.Sprintf("Task %d", i),
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

		// Create pending job directly (time.Time{} = schedule immediately)
		_, err = store.ScheduleGenerationJob(ctx, templateID, time.Time{},
			time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
		require.NoError(t, err)
	}

	// Launch 3 workers
	numWorkers := 3
	var wg sync.WaitGroup
	workerJobCounts := make([]atomic.Int32, numWorkers)

	for i := range numWorkers {
		workerIdx := i

		wg.Go(func() {
			genWorker := setupGenerationWorkerTest(t, store)

			// Process jobs until queue is empty
			for {
				// Count completed jobs before processing
				var beforeCount int
				store.Pool().QueryRow(ctx, `
					SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'completed'
				`).Scan(&beforeCount)

				err := genWorker.RunProcessOnce(ctx)
				if err != nil {
					t.Logf("Worker %d error: %v", workerIdx, err)
					return
				}

				// Count completed jobs after processing
				var afterCount int
				store.Pool().QueryRow(ctx, `
					SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'completed'
				`).Scan(&afterCount)

				// If a job was completed, increment this worker's counter
				if afterCount > beforeCount {
					workerJobCounts[workerIdx].Add(1)
				}

				// Check if there are any remaining pending/running jobs
				var remainingCount int
				store.Pool().QueryRow(ctx, `
					SELECT COUNT(*) FROM recurring_generation_jobs
					WHERE status IN ('pending', 'running') AND scheduled_for <= NOW()
				`).Scan(&remainingCount)

				if remainingCount == 0 {
					// No more jobs available
					return
				}
			}
		})
	}

	// Wait for all workers with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("Test timeout - workers did not complete in time")
	}

	// Verify all jobs were completed in DB (not worker count which may include "no job available" calls)
	var completedInDB int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'completed'
	`).Scan(&completedInDB)
	require.NoError(t, err)
	assert.Equal(t, numTemplates, completedInDB,
		"All jobs should be completed exactly once in database")

	// Verify distribution (each worker should get some jobs)
	for i := range numWorkers {
		count := workerJobCounts[i].Load()
		t.Logf("Worker %d processed %d jobs", i, count)
		assert.Greater(t, count, int32(0), "Each worker should process at least one job")
	}

	// Verify all jobs are completed in DB
	var completedJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'completed'
	`).Scan(&completedJobs)
	require.NoError(t, err)
	assert.Equal(t, numTemplates, completedJobs)
}

// TestWorker_MultipleWorkers_HighLoad tests high-load scenario with 500+ templates.
func TestWorker_MultipleWorkers_HighLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high-load test in short mode")
	}

	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate list UUID")
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "High Load Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 500 templates with mix of patterns
	totalTemplates := 500
	patterns := []struct {
		pattern domain.RecurrencePattern
		config  map[string]any
		count   int
		window  int
	}{
		{domain.RecurrenceDaily, map[string]any{"interval": float64(1)}, 200, 15},
		{domain.RecurrenceWeekly, map[string]any{"interval": float64(1)}, 150, 60},
		{domain.RecurrenceMonthly, map[string]any{"interval": float64(1)}, 100, 180},
		{domain.RecurrenceWeekdays, map[string]any{}, 50, 30},
	}

	templateCount := 0
	for _, p := range patterns {
		for i := 0; i < p.count; i++ {
			templateUUID, err := uuid.NewV7()
			require.NoError(t, err, "failed to generate template UUID")
			templateID := templateUUID.String()

			template := &domain.RecurringTemplate{
				ID:                    templateID,
				ListID:                listID,
				Title:                 fmt.Sprintf("Load Task %d", templateCount),
				RecurrencePattern:     p.pattern,
				RecurrenceConfig:      p.config,
				SyncHorizonDays:       14,
				GenerationHorizonDays: 365,
				IsActive:              true,
				CreatedAt:             time.Now().UTC(),
				UpdatedAt:             time.Now().UTC(),
			}

			_, err = store.CreateRecurringTemplate(ctx, template)
			require.NoError(t, err)

			// Create pending job (time.Time{} = schedule immediately)
			_, err = store.ScheduleGenerationJob(ctx, templateID, time.Time{},
				time.Now().UTC(), time.Now().UTC().AddDate(0, 0, p.window))
			require.NoError(t, err)

			templateCount++
		}
	}

	assert.Equal(t, totalTemplates, templateCount, "Should create all templates")

	// Launch 10 workers
	numWorkers := 10
	var wg sync.WaitGroup
	startTime := time.Now().UTC()

	for i := range numWorkers {
		workerID := i

		wg.Go(func() {
			genWorker := setupGenerationWorkerTest(t, store)
			localCount := 0

			for {
				err := genWorker.RunProcessOnce(ctx)
				if err != nil {
					t.Logf("Worker %d error after %d jobs: %v", workerID, localCount, err)
					return
				}

				// Check if there are any remaining pending/running jobs
				var remainingCount int
				store.Pool().QueryRow(ctx, `
					SELECT COUNT(*) FROM recurring_generation_jobs
					WHERE status IN ('pending', 'running') AND scheduled_for <= NOW()
				`).Scan(&remainingCount)

				if remainingCount == 0 {
					t.Logf("Worker %d finished after checking (processed some jobs)", workerID)
					return
				}

				localCount++
			}
		})
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		duration := time.Since(startTime)
		t.Logf("Completed %d jobs in %v", totalTemplates, duration)
		t.Logf("Throughput: %.2f jobs/sec", float64(totalTemplates)/duration.Seconds())
	case <-time.After(5 * time.Minute):
		t.Fatal("High-load test timeout")
	}

	// Verify all jobs completed in database
	var completedInDB int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'completed'
	`).Scan(&completedInDB)
	require.NoError(t, err)
	assert.Equal(t, totalTemplates, completedInDB,
		"All %d jobs should be completed in database", totalTemplates)

	// Verify task counts
	itemFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	itemsResult, err := store.FindItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: itemFilter, Limit: 10000}, nil)
	require.NoError(t, err)

	taskCount := len(itemsResult.Items)
	t.Logf("Generated %d total tasks", taskCount)

	// Rough estimate: should have thousands of tasks
	assert.Greater(t, taskCount, 5000, "Should generate many tasks")
	assert.Less(t, taskCount, 7000, "Should not generate excessive tasks")
}

// TestWorker_NoJobsAvailable tests worker behavior when queue is empty.
func TestWorker_NoJobsAvailable(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Process with empty queue using GenerationWorker
	genWorker := setupGenerationWorkerTest(t, store)
	err := genWorker.RunProcessOnce(ctx)
	require.NoError(t, err, "Should not error when no jobs available")
}

// TestWorker_GenerationWindow tests generation window advancement.
func TestWorker_GenerationWindow(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate list UUID")
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Window Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create template with specific generated_through
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate template UUID")
	templateID := templateUUID.String()
	lastGenerated := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Window Test Task",
		RecurrencePattern:     domain.RecurrenceMonthly,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 365,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create job with specific generation range (time.Time{} = schedule immediately)
	targetDate := time.Now().UTC().AddDate(0, 0, 180)
	_, err = store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		lastGenerated, targetDate)
	require.NoError(t, err)

	// Process job using GenerationWorker
	genWorker := setupGenerationWorkerTest(t, store)
	err = genWorker.RunProcessOnce(ctx)
	require.NoError(t, err)

	// Verify tasks start from lastGenerated date
	itemFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	itemsResult, err := store.FindItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: itemFilter, Limit: 1000}, nil)
	require.NoError(t, err)

	if len(itemsResult.Items) > 0 {
		firstTask := itemsResult.Items[0]

		// First task should be at or after the last generated date
		if firstTask.DueAt != nil {
			assert.True(t, firstTask.DueAt.After(lastGenerated) || firstTask.DueAt.Equal(lastGenerated),
				"First task should be after last generated date")
		}
	}

	// Verify template window was updated

	var pendingJobs int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1 AND status = 'pending'
	`, templateID).Scan(&pendingJobs)
	require.NoError(t, err)
	assert.Equal(t, 0, pendingJobs, "Should not create duplicate job")
}

// TestWorker_PreservesExistingItemsAndHistory tests that the worker doesn't destroy
// existing items and their status history when generating new recurring tasks.
func TestWorker_PreservesExistingItemsAndHistory(t *testing.T) {
	store, w, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate list UUID")
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "History Preservation Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an existing task with status history
	existingTaskUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate task UUID")
	existingTaskID := existingTaskUUID.String()
	existingTask := domain.TodoItem{
		ID:        existingTaskID,
		Title:     "Existing Task - Do Not Delete",
		Status:    domain.TaskStatusTodo,
		CreatedAt: time.Now().UTC().Add(-24 * time.Hour), // Created yesterday
		UpdatedAt: time.Now().UTC().Add(-24 * time.Hour),
	}
	_, err = store.CreateItem(ctx, listID, &existingTask)
	require.NoError(t, err)

	// Update task status to create status history
	existingTask.Status = domain.TaskStatusInProgress
	existingTask.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)
	_, err = store.UpdateItem(ctx, ItemToUpdateParams(listID, &existingTask))
	require.NoError(t, err)

	existingTask.Status = domain.TaskStatusDone
	existingTask.UpdatedAt = time.Now().UTC()
	_, err = store.UpdateItem(ctx, ItemToUpdateParams(listID, &existingTask))
	require.NoError(t, err)

	// Verify status history was created (should have 3 entries: TODO, IN_PROGRESS, DONE)
	var historyCount int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, existingTaskID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 3, historyCount, "Existing task should have 3 status history entries")

	// Create recurring template for the same list
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate template UUID")
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "Recurring Task",
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

	// Run worker to generate recurring tasks
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	// Process job using GenerationWorker
	genWorker := setupGenerationWorkerTest(t, store)
	err = genWorker.RunProcessOnce(ctx)
	require.NoError(t, err)

	// Verify existing task still exists
	itemFilter, err := domain.NewItemsFilter(domain.ItemsFilterInput{})
	require.NoError(t, err)
	itemsResult, err := store.FindItems(ctx, domain.ListTasksParams{ListID: &listID, Filter: itemFilter, Limit: 1000}, nil)
	require.NoError(t, err)

	var foundExisting bool
	var foundRecurring int
	for _, item := range itemsResult.Items {
		if item.ID == existingTaskID {
			foundExisting = true
			assert.Equal(t, "Existing Task - Do Not Delete", item.Title)
			assert.Equal(t, domain.TaskStatusDone, item.Status)
		}
		if item.RecurringTemplateID != nil && *item.RecurringTemplateID == templateID {
			foundRecurring++
		}
	}

	assert.True(t, foundExisting, "Existing task should still be in the list")
	assert.Greater(t, foundRecurring, 0, "Should have generated recurring tasks")

	// CRITICAL: Verify status history was preserved
	var historyCountAfter int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, existingTaskID).Scan(&historyCountAfter)
	require.NoError(t, err)
	assert.Equal(t, 3, historyCountAfter, "Existing task's status history should be preserved")

	// Verify we can still query the history
	var statusHistory []string
	rows, err := store.Pool().Query(ctx, `
		SELECT to_status FROM task_status_history
		WHERE task_id = $1
		ORDER BY changed_at ASC
	`, existingTaskID)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var status string
		err = rows.Scan(&status)
		require.NoError(t, err)
		statusHistory = append(statusHistory, status)
	}

	assert.Equal(t, []string{"todo", "in_progress", "done"}, statusHistory,
		"Status history should show progression: todo → in_progress → done")

	t.Logf("✓ Existing task preserved with %d history entries", historyCountAfter)
	t.Logf("✓ Generated %d new recurring tasks", foundRecurring)
	t.Logf("✓ Total items in list: %d", len(itemsResult.Items))
}

// TestCoordinator_ExhaustedRetries_MovesToDeadLetter tests that jobs exceeding
// max retries are atomically moved to the dead letter queue.
func TestCoordinator_ExhaustedRetries_MovesToDeadLetter(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "DLQ Test",
		CreatedAt: time.Now().UTC(),
	}
	_, err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create template
	templateUUID, err := uuid.NewV7()
	require.NoError(t, err)
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                    templateID,
		ListID:                listID,
		Title:                 "DLQ Test Task",
		RecurrencePattern:     domain.RecurrenceDaily,
		RecurrenceConfig:      map[string]any{"interval": float64(1)},
		SyncHorizonDays:       14,
		GenerationHorizonDays: 30,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create job
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Manually set retry_count to max (simulating already-failed job)
	maxRetries := 3
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET retry_count = $1
		WHERE id = $2
	`, maxRetries, jobID)
	require.NoError(t, err)

	// Create coordinator for job operations
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Claim the job
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Fail the job (should exhaust retries and move to DLQ)
	willRetry, err := coordinator.FailJob(ctx, job.ID, cfg.WorkerID, "test error", cfg.RetryConfig)
	require.NoError(t, err)
	assert.False(t, willRetry, "Job should not retry after exhausting max retries")

	// Verify job moved to dead letter queue
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1, "Should have exactly one job in dead letter queue")

	dlJob := dlJobs[0]
	assert.Equal(t, jobID, dlJob.OriginalJobID)
	assert.Equal(t, "exhausted", dlJob.ErrorType)
	assert.Contains(t, dlJob.ErrorMessage, "test error")
	assert.Equal(t, maxRetries+1, dlJob.RetryCount) // Should be max+1
	assert.Equal(t, cfg.WorkerID, dlJob.LastWorkerID, "LastWorkerID should track which worker exhausted the retries")

	// Verify job is marked as discarded in original table
	var status string
	err = store.Pool().QueryRow(ctx, `
		SELECT status FROM recurring_generation_jobs WHERE id = $1
	`, jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "discarded", status)
}

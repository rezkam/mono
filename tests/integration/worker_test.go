package integration_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/core"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/rezkam/mono/internal/storage/sql/repository"
	"github.com/rezkam/mono/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWorkerTest initializes a test environment with storage and worker.
func setupWorkerTest(t *testing.T) (*repository.Store, *worker.Worker, func()) {
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		t.Skip("TEST_POSTGRES_URL not set, skipping worker tests")
	}

	ctx := context.Background()
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	// Clean up tables before test
	_, err = store.DB().Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	w := worker.New(store)

	cleanup := func() {
		store.DB().Exec("TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		store.Close()
	}

	return store, w, cleanup
}

// TestWorker_CompleteFlow tests the end-to-end flow with multiple recurrence patterns.
func TestWorker_CompleteFlow(t *testing.T) {
	store, w, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test cases with different patterns
	testCases := []struct {
		name             string
		pattern          core.RecurrencePattern
		config           map[string]interface{}
		generationWindow int
		expectedMinTasks int // Minimum tasks expected
		expectedMaxTasks int // Maximum tasks expected
	}{
		{
			name:             "Daily",
			pattern:          core.RecurrenceDaily,
			config:           map[string]interface{}{"interval": float64(1)},
			generationWindow: 30,
			expectedMinTasks: 30,
			expectedMaxTasks: 31, // Account for edge cases
		},
		{
			name:             "Weekly",
			pattern:          core.RecurrenceWeekly,
			config:           map[string]interface{}{"interval": float64(1)},
			generationWindow: 90,
			expectedMinTasks: 12,
			expectedMaxTasks: 14,
		},
		{
			name:             "Monthly",
			pattern:          core.RecurrenceMonthly,
			config:           map[string]interface{}{"interval": float64(1)},
			generationWindow: 365,
			expectedMinTasks: 11,
			expectedMaxTasks: 13,
		},
		{
			name:             "Weekdays",
			pattern:          core.RecurrenceWeekdays,
			config:           map[string]interface{}{},
			generationWindow: 30,
			expectedMinTasks: 21,
			expectedMaxTasks: 23,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up any leftover templates/jobs from previous subtests
			_, err := store.DB().Exec("DELETE FROM recurring_generation_jobs")
			require.NoError(t, err)
			_, err = store.DB().Exec("DELETE FROM recurring_task_templates")
			require.NoError(t, err)

			// Create fresh list for this subtest
			listID := uuid.New().String()
			list := &core.TodoList{
				ID:         listID,
				Title:      fmt.Sprintf("Test List - %s", tc.name),
				Items:      []core.TodoItem{},
				CreateTime: time.Now(),
			}
			err = store.CreateList(ctx, list)
			require.NoError(t, err)

			// Create template
			templateID := uuid.New().String()
			template := &core.RecurringTaskTemplate{
				ID:                   templateID,
				ListID:               listID,
				Title:                fmt.Sprintf("Test Task - %s", tc.name),
				RecurrencePattern:    tc.pattern,
				RecurrenceConfig:     tc.config,
				GenerationWindowDays: tc.generationWindow,
				IsActive:             true,
				CreatedAt:            time.Now(),
				UpdatedAt:            time.Now(),
			}
			err = store.CreateRecurringTemplate(ctx, template)
			require.NoError(t, err)

			// Step 1: Schedule job
			err = w.RunScheduleOnce(ctx)
			require.NoError(t, err)

			// Verify job was created
			var jobCount int
			err = store.DB().QueryRow(`
				SELECT COUNT(*) FROM recurring_generation_jobs
				WHERE template_id = $1 AND status = 'PENDING'
			`, template.ID).Scan(&jobCount)
			require.NoError(t, err)
			assert.Equal(t, 1, jobCount, "Should have created one pending job")

			// Step 2: Process job
			processed, err := w.RunProcessOnce(ctx)
			require.NoError(t, err)
			assert.True(t, processed, "Should have processed a job")

			// Verify job completed
			var completedCount int
			err = store.DB().QueryRow(`
				SELECT COUNT(*) FROM recurring_generation_jobs
				WHERE template_id = $1 AND status = 'COMPLETED'
			`, template.ID).Scan(&completedCount)
			require.NoError(t, err)
			assert.Equal(t, 1, completedCount, "Job should be completed")

			// Verify tasks were created
			updatedList, err := store.GetList(ctx, listID)
			require.NoError(t, err)

			taskCount := len(updatedList.Items)
			assert.GreaterOrEqual(t, taskCount, tc.expectedMinTasks,
				"Should have at least %d tasks", tc.expectedMinTasks)
			assert.LessOrEqual(t, taskCount, tc.expectedMaxTasks,
				"Should have at most %d tasks", tc.expectedMaxTasks)

			// Verify tasks span into the future
			if taskCount > 0 {
				firstTask := updatedList.Items[0]
				lastTask := updatedList.Items[taskCount-1]

				if lastTask.DueTime != nil && firstTask.DueTime != nil {
					assert.True(t, lastTask.DueTime.After(*firstTask.DueTime),
						"Tasks should span across time")

					// For longer windows, verify substantial time span
					if tc.generationWindow >= 90 {
						daysBetween := lastTask.DueTime.Sub(*firstTask.DueTime).Hours() / 24
						assert.Greater(t, daysBetween, 60.0,
							"Tasks should span at least 60 days for %s pattern", tc.pattern)
					}
				}
			}

			// Verify template was updated
			updatedTemplate, err := store.GetRecurringTemplate(ctx, template.ID)
			require.NoError(t, err)
			assert.False(t, updatedTemplate.LastGeneratedUntil.IsZero(),
				"LastGeneratedUntil should be updated")
		})
	}
}

// TestWorker_MultipleWorkers_JobDistribution tests multiple workers processing jobs without conflicts.
func TestWorker_MultipleWorkers_JobDistribution(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listID := uuid.New().String()
	list := &core.TodoList{
		ID:         listID,
		Title:      "Distribution Test",
		Items:      []core.TodoItem{},
		CreateTime: time.Now(),
	}
	err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 10 templates
	numTemplates := 10
	templateIDs := make([]string, numTemplates)
	for i := 0; i < numTemplates; i++ {
		templateID := uuid.New().String()
		templateIDs[i] = templateID

		template := &core.RecurringTaskTemplate{
			ID:                   templateID,
			ListID:               listID,
			Title:                fmt.Sprintf("Task %d", i),
			RecurrencePattern:    core.RecurrenceDaily,
			RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
			GenerationWindowDays: 7,
			IsActive:             true,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		err = store.CreateRecurringTemplate(ctx, template)
		require.NoError(t, err)

		// Create pending job directly (time.Time{} = schedule immediately)
		_, err = store.CreateGenerationJob(ctx, templateID, time.Time{},
			time.Now(), time.Now().AddDate(0, 0, 7))
		require.NoError(t, err)
	}

	// Launch 3 workers
	numWorkers := 3
	var wg sync.WaitGroup
	var processedCount atomic.Int32
	workerJobCounts := make([]atomic.Int32, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerIdx := i

		go func() {
			defer wg.Done()

			w := worker.New(store)

			// Process jobs until queue is empty
			for {
				processed, err := w.RunProcessOnce(ctx)
				if err != nil {
					t.Logf("Worker %d error: %v", workerIdx, err)
					return
				}

				if !processed {
					// No more jobs
					return
				}

				processedCount.Add(1)
				workerJobCounts[workerIdx].Add(1)
			}
		}()
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

	// Verify all jobs were completed
	totalProcessed := processedCount.Load()
	assert.Equal(t, int32(numTemplates), totalProcessed,
		"All jobs should be processed exactly once")

	// Verify distribution (each worker should get some jobs)
	for i := 0; i < numWorkers; i++ {
		count := workerJobCounts[i].Load()
		t.Logf("Worker %d processed %d jobs", i, count)
		assert.Greater(t, count, int32(0), "Each worker should process at least one job")
	}

	// Verify all jobs are completed in DB
	var completedJobs int
	err = store.DB().QueryRow(`
		SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'COMPLETED'
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
	listID := uuid.New().String()
	list := &core.TodoList{
		ID:         listID,
		Title:      "High Load Test",
		Items:      []core.TodoItem{},
		CreateTime: time.Now(),
	}
	err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create 500 templates with mix of patterns
	totalTemplates := 500
	patterns := []struct {
		pattern core.RecurrencePattern
		config  map[string]interface{}
		count   int
		window  int
	}{
		{core.RecurrenceDaily, map[string]interface{}{"interval": float64(1)}, 200, 15},
		{core.RecurrenceWeekly, map[string]interface{}{"interval": float64(1)}, 150, 60},
		{core.RecurrenceMonthly, map[string]interface{}{"interval": float64(1)}, 100, 180},
		{core.RecurrenceWeekdays, map[string]interface{}{}, 50, 30},
	}

	templateCount := 0
	for _, p := range patterns {
		for i := 0; i < p.count; i++ {
			templateID := uuid.New().String()

			template := &core.RecurringTaskTemplate{
				ID:                   templateID,
				ListID:               listID,
				Title:                fmt.Sprintf("Load Task %d", templateCount),
				RecurrencePattern:    p.pattern,
				RecurrenceConfig:     p.config,
				GenerationWindowDays: p.window,
				IsActive:             true,
				CreatedAt:            time.Now(),
				UpdatedAt:            time.Now(),
			}

			err = store.CreateRecurringTemplate(ctx, template)
			require.NoError(t, err)

			// Create pending job (time.Time{} = schedule immediately)
			_, err = store.CreateGenerationJob(ctx, templateID, time.Time{},
				time.Now(), time.Now().AddDate(0, 0, p.window))
			require.NoError(t, err)

			templateCount++
		}
	}

	assert.Equal(t, totalTemplates, templateCount, "Should create all templates")

	// Launch 10 workers
	numWorkers := 10
	var wg sync.WaitGroup
	var processedCount atomic.Int32
	startTime := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			w := worker.New(store)
			localCount := 0

			for {
				processed, err := w.RunProcessOnce(ctx)
				if err != nil {
					t.Logf("Worker %d error: %v", workerID, err)
					return
				}

				if !processed {
					t.Logf("Worker %d completed %d jobs", workerID, localCount)
					return
				}

				processedCount.Add(1)
				localCount++
			}
		}(i)
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

	// Verify all jobs completed
	assert.Equal(t, int32(totalTemplates), processedCount.Load())

	// Verify task counts
	updatedList, err := store.GetList(ctx, listID)
	require.NoError(t, err)

	taskCount := len(updatedList.Items)
	t.Logf("Generated %d total tasks", taskCount)

	// Rough estimate: should have thousands of tasks
	assert.Greater(t, taskCount, 5000, "Should generate many tasks")
	assert.Less(t, taskCount, 7000, "Should not generate excessive tasks")
}

// TestWorker_NoJobsAvailable tests worker behavior when queue is empty.
func TestWorker_NoJobsAvailable(t *testing.T) {
	_, w, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Process with empty queue
	processed, err := w.RunProcessOnce(ctx)
	require.NoError(t, err)
	assert.False(t, processed, "Should return false when no jobs available")
}

// TestWorker_GenerationWindow tests generation window advancement.
func TestWorker_GenerationWindow(t *testing.T) {
	store, w, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test list
	listID := uuid.New().String()
	list := &core.TodoList{
		ID:         listID,
		Title:      "Window Test",
		Items:      []core.TodoItem{},
		CreateTime: time.Now(),
	}
	err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create template with specific last_generated_until
	templateID := uuid.New().String()
	lastGenerated := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	template := &core.RecurringTaskTemplate{
		ID:                   templateID,
		ListID:               listID,
		Title:                "Window Test Task",
		RecurrencePattern:    core.RecurrenceMonthly,
		RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
		GenerationWindowDays: 180,
		LastGeneratedUntil:   lastGenerated,
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Create job with specific generation range (time.Time{} = schedule immediately)
	targetDate := time.Now().AddDate(0, 0, 180)
	_, err = store.CreateGenerationJob(ctx, templateID, time.Time{},
		lastGenerated, targetDate)
	require.NoError(t, err)

	// Process job
	processed, err := w.RunProcessOnce(ctx)
	require.NoError(t, err)
	assert.True(t, processed)

	// Verify tasks start from lastGenerated date
	updatedList, err := store.GetList(ctx, listID)
	require.NoError(t, err)

	if len(updatedList.Items) > 0 {
		firstTask := updatedList.Items[0]

		// First task should be at or after the last generated date
		if firstTask.DueTime != nil {
			assert.True(t, firstTask.DueTime.After(lastGenerated) || firstTask.DueTime.Equal(lastGenerated),
				"First task should be after last generated date")
		}
	}

	// Verify template window was updated
	updatedTemplate, err := store.GetRecurringTemplate(ctx, templateID)
	require.NoError(t, err)
	assert.True(t, updatedTemplate.LastGeneratedUntil.After(lastGenerated),
		"LastGeneratedUntil should be updated")

	// Run schedule again - should not create duplicate job since window is covered
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	var pendingJobs int
	err = store.DB().QueryRow(`
		SELECT COUNT(*) FROM recurring_generation_jobs
		WHERE template_id = $1 AND status = 'PENDING'
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
	listID := uuid.New().String()
	list := &core.TodoList{
		ID:         listID,
		Title:      "History Preservation Test",
		Items:      []core.TodoItem{},
		CreateTime: time.Now(),
	}
	err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create an existing task with status history
	existingTaskID := uuid.New().String()
	existingTask := core.TodoItem{
		ID:         existingTaskID,
		Title:      "Existing Task - Do Not Delete",
		Status:     core.TaskStatusTodo,
		CreateTime: time.Now().Add(-24 * time.Hour), // Created yesterday
		UpdatedAt:  time.Now().Add(-24 * time.Hour),
	}
	err = store.CreateTodoItem(ctx, listID, existingTask)
	require.NoError(t, err)

	// Update task status to create status history
	existingTask.Status = core.TaskStatusInProgress
	existingTask.UpdatedAt = time.Now().Add(-1 * time.Hour)
	err = store.UpdateTodoItem(ctx, existingTask)
	require.NoError(t, err)

	existingTask.Status = core.TaskStatusDone
	existingTask.UpdatedAt = time.Now()
	err = store.UpdateTodoItem(ctx, existingTask)
	require.NoError(t, err)

	// Verify status history was created (should have 3 entries: TODO, IN_PROGRESS, DONE)
	var historyCount int
	err = store.DB().QueryRow(`
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, existingTaskID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 3, historyCount, "Existing task should have 3 status history entries")

	// Create recurring template for the same list
	templateID := uuid.New().String()
	template := &core.RecurringTaskTemplate{
		ID:                   templateID,
		ListID:               listID,
		Title:                "Recurring Task",
		RecurrencePattern:    core.RecurrenceDaily,
		RecurrenceConfig:     map[string]interface{}{"interval": float64(1)},
		GenerationWindowDays: 7,
		IsActive:             true,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Run worker to generate recurring tasks
	err = w.RunScheduleOnce(ctx)
	require.NoError(t, err)

	processed, err := w.RunProcessOnce(ctx)
	require.NoError(t, err)
	assert.True(t, processed, "Should process the job")

	// Verify existing task still exists
	updatedList, err := store.GetList(ctx, listID)
	require.NoError(t, err)

	var foundExisting bool
	var foundRecurring int
	for _, item := range updatedList.Items {
		if item.ID == existingTaskID {
			foundExisting = true
			assert.Equal(t, "Existing Task - Do Not Delete", item.Title)
			assert.Equal(t, core.TaskStatusDone, item.Status)
		}
		if item.RecurringTemplateID != nil && *item.RecurringTemplateID == templateID {
			foundRecurring++
		}
	}

	assert.True(t, foundExisting, "Existing task should still be in the list")
	assert.Greater(t, foundRecurring, 0, "Should have generated recurring tasks")

	// CRITICAL: Verify status history was preserved
	var historyCountAfter int
	err = store.DB().QueryRow(`
		SELECT COUNT(*) FROM task_status_history WHERE task_id = $1
	`, existingTaskID).Scan(&historyCountAfter)
	require.NoError(t, err)
	assert.Equal(t, 3, historyCountAfter, "Existing task's status history should be preserved")

	// Verify we can still query the history
	var statusHistory []string
	rows, err := store.DB().Query(`
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

	assert.Equal(t, []string{"TODO", "IN_PROGRESS", "DONE"}, statusHistory,
		"Status history should show progression: TODO → IN_PROGRESS → DONE")

	t.Logf("✓ Existing task preserved with %d history entries", historyCountAfter)
	t.Logf("✓ Generated %d new recurring tasks", foundRecurring)
	t.Logf("✓ Total items in list: %d", len(updatedList.Items))
}

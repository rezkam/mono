package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rezkam/mono/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentJobScheduling_PreventsDuplicates verifies that concurrent job scheduling
// for the same template produces exactly ONE active job, not duplicates.
//
// This is a TDD test for the missing unique constraint on recurring_generation_jobs.
// Currently (before fix): Test FAILS - multiple jobs are created.
// After fix: Test PASSES - only one job is created.
//
// The race condition being tested:
//
//	Timeline     Worker A                          Worker B
//	─────────────────────────────────────────────────────────────────
//	T1           HasPendingOrRunningJob() → false
//	T2                                             HasPendingOrRunningJob() → false
//	T3           InsertGenerationJob() → ✓
//	T4                                             InsertGenerationJob() → ✓ (DUPLICATE!)
//
// The fix: Add a partial unique index on template_id WHERE status IN ('pending', 'scheduled', 'running')
func TestConcurrentJobScheduling_PreventsDuplicates(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create a list first
	listID := uuid.Must(uuid.NewV7()).String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Test List for Job Concurrency",
		CreatedAt: time.Now().UTC(),
	}
	_, err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template
	templateID := uuid.Must(uuid.NewV7()).String()
	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Template for Concurrent Job Test",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	// Schedule parameters
	now := time.Now().UTC()
	from := now
	until := now.AddDate(0, 0, 30) // 30 days ahead

	const numGoroutines = 10

	// Channel to collect results
	type result struct {
		jobID string
		err   error
	}
	results := make(chan result, numGoroutines)

	// Barrier to maximize race window - all goroutines start simultaneously
	var startBarrier sync.WaitGroup
	startBarrier.Add(1)

	var wg sync.WaitGroup

	for range numGoroutines {
		wg.Go(func() {
			// Wait for all goroutines to be ready
			startBarrier.Wait()

			// Try to schedule a job for the same template
			jobID, err := store.ScheduleGenerationJob(ctx, templateID, now, from, until)
			results <- result{jobID: jobID, err: err}
		})
	}

	// Release all goroutines at once to maximize race condition
	startBarrier.Done()

	// Wait for all to complete
	wg.Wait()
	close(results)

	// Count successful job creations
	var successCount int
	var createdJobIDs []string
	var errors []error

	for r := range results {
		if r.err == nil && r.jobID != "" {
			successCount++
			createdJobIDs = append(createdJobIDs, r.jobID)
		} else if r.err != nil {
			errors = append(errors, r.err)
		}
	}

	// Query the actual number of jobs in the database for this template
	var actualJobCount int
	err = store.Pool().QueryRow(ctx,
		`SELECT COUNT(*) FROM recurring_generation_jobs
		 WHERE template_id = $1 AND status IN ('pending', 'scheduled', 'running')`,
		templateID).Scan(&actualJobCount)
	require.NoError(t, err)

	// THE KEY ASSERTION:
	// With proper unique constraint: exactlyONE job should exist
	// Without unique constraint (current bug): multiple jobs may exist
	assert.Equal(t, 1, actualJobCount,
		"Expected exactly 1 active job for template, but found %d. "+
			"This indicates the unique constraint is missing. "+
			"Successfully created jobs: %d, Errors: %v",
		actualJobCount, successCount, errors)

	t.Logf("Results: %d successful inserts, %d actual jobs in DB, %d errors",
		successCount, actualJobCount, len(errors))

	// Additional assertion: if we have more than 1 job, log the duplicates for debugging
	if actualJobCount > 1 {
		var jobStatuses []string
		rows, err := store.Pool().Query(ctx,
			`SELECT id, status, created_at FROM recurring_generation_jobs
			 WHERE template_id = $1 ORDER BY created_at`,
			templateID)
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var id, status string
			var createdAt time.Time
			err := rows.Scan(&id, &status, &createdAt)
			require.NoError(t, err)
			jobStatuses = append(jobStatuses, id+" ("+status+")")
		}
		t.Logf("Duplicate jobs found: %v", jobStatuses)
	}
}

// TestConcurrentJobScheduling_AllowsSequentialJobs verifies that after completing
// a job, a new job CAN be scheduled for the same template (not blocked forever).
func TestConcurrentJobScheduling_AllowsSequentialJobs(t *testing.T) {
	store, ctx := SetupTestStore(t)

	// Create a list first
	listID := uuid.Must(uuid.NewV7()).String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Test List for Sequential Jobs",
		CreatedAt: time.Now().UTC(),
	}
	_, err := store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create a template
	templateID := uuid.Must(uuid.NewV7()).String()
	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Template for Sequential Job Test",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{},
		IsActive:          true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
		GeneratedThrough:  time.Now().UTC(),
	}
	_, err = store.CreateRecurringTemplate(ctx, template)
	require.NoError(t, err)

	now := time.Now().UTC()

	// Schedule first job
	jobID1, err := store.ScheduleGenerationJob(ctx, templateID, now, now, now.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.NotEmpty(t, jobID1)

	// Mark first job as completed
	_, err = store.Pool().Exec(ctx,
		`UPDATE recurring_generation_jobs SET status = 'completed', completed_at = NOW() WHERE id = $1`,
		jobID1)
	require.NoError(t, err)

	// Should now be able to schedule a NEW job (since old one is completed, not active)
	jobID2, err := store.ScheduleGenerationJob(ctx, templateID, now, now.AddDate(0, 0, 7), now.AddDate(0, 0, 14))
	require.NoError(t, err)
	assert.NotEmpty(t, jobID2)
	assert.NotEqual(t, jobID1, jobID2, "Second job should have different ID")

	// Verify we have 1 active job and 1 completed job
	var activeCount, completedCount int
	err = store.Pool().QueryRow(ctx,
		`SELECT COUNT(*) FROM recurring_generation_jobs WHERE template_id = $1 AND status IN ('pending', 'scheduled', 'running')`,
		templateID).Scan(&activeCount)
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount, "Should have exactly 1 active job")

	err = store.Pool().QueryRow(ctx,
		`SELECT COUNT(*) FROM recurring_generation_jobs WHERE template_id = $1 AND status = 'completed'`,
		templateID).Scan(&completedCount)
	require.NoError(t, err)
	assert.Equal(t, 1, completedCount, "Should have exactly 1 completed job")
}

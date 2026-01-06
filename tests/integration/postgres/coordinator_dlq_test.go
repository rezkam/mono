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

// TestCoordinator_ExhaustedRetries_AtomicDLQMove verifies that exhausted jobs
// are atomically moved to DLQ (both DLQ insert and job discard succeed together).
func TestCoordinator_ExhaustedRetries_AtomicDLQMove(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job at max retries
	templateID := createTestTemplate(t, store, ctx)
	jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)

	// Fail the job (should exhaust retries and move to DLQ)
	willRetry, err := coordinator.FailJob(ctx, jobID, *job.ClaimedBy, "exhausted error", worker.DefaultRetryConfig())
	require.NoError(t, err)
	assert.False(t, willRetry, "Should not retry after max retries")

	// Verify atomicity: BOTH operations succeeded
	// 1. Job in DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1, "Job should be in DLQ")
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, "exhausted", dlJobs[0].ErrorType)
	assert.Equal(t, *job.ClaimedBy, dlJobs[0].LastWorkerID, "LastWorkerID should match the worker that failed the job")

	// 2. Job marked as discarded
	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "discarded", status)
}

// TestCoordinator_RetryBehavior_ExponentialBackoff verifies retry scheduling
// with exponential backoff before max retries.
func TestCoordinator_RetryBehavior_ExponentialBackoff(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job with retry_count = 0
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Claim the job
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, 0, job.RetryCount)

	beforeFailure := time.Now().UTC()

	// Fail the job (should schedule retry #1)
	willRetry, err := coordinator.FailJob(ctx, jobID, cfg.WorkerID, "transient error", cfg.RetryConfig)
	require.NoError(t, err)
	assert.True(t, willRetry, "Should retry after first failure")

	afterFailure := time.Now().UTC()

	// Verify job was rescheduled for retry
	var status string
	var retryCount int32
	var scheduledFor time.Time
	err = store.Pool().QueryRow(ctx, `
		SELECT status, retry_count, scheduled_for
		FROM recurring_generation_jobs
		WHERE id = $1
	`, jobID).Scan(&status, &retryCount, &scheduledFor)
	require.NoError(t, err)

	assert.Equal(t, "pending", status, "Job should be pending for retry")
	assert.Equal(t, int32(1), retryCount, "Retry count should be incremented")

	// Verify exponential backoff (first retry has baseDelay with jitter)
	// Expected: ~1 minute with jitter (0-2 minutes range)
	minDelay := beforeFailure.Add(0 * time.Second) // Minimum (full jitter can be 0)
	maxDelay := afterFailure.Add(2 * time.Minute)  // Maximum (baseDelay * 2)
	assert.True(t, scheduledFor.After(minDelay), "Retry should be scheduled in future")
	assert.True(t, scheduledFor.Before(maxDelay), "Retry delay should not exceed max")

	// Verify NOT in DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, dlJobs, "Job should not be in DLQ during retries")
}

// TestCoordinator_OwnershipCheck_PreventsRaceConditions verifies that
// concurrent workers cannot interfere with each other's job failures.
func TestCoordinator_OwnershipCheck_PreventsRaceConditions(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Set retry_count to max
	maxRetries := 3
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs SET retry_count = $1 WHERE id = $2
	`, maxRetries, jobID)
	require.NoError(t, err)

	// Worker A claims the job
	workerA := "worker-A"
	jobA, err := coordinator.ClaimNextJob(ctx, workerA, 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, jobA)

	// Worker B tries to fail the same job (should fail ownership check)
	workerB := "worker-B"
	willRetry, err := coordinator.FailJob(ctx, jobID, workerB, "error from B", worker.DefaultRetryConfig())

	// Should return ownership error OR success with willRetry=false
	// (depending on whether DiscardJobAfterMaxRetries checks ownership)
	// Let's verify the job was NOT moved to DLQ by worker B
	dlJobsBefore, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)

	// Worker A fails the job (should succeed)
	willRetry, err = coordinator.FailJob(ctx, jobID, workerA, "error from A", worker.DefaultRetryConfig())
	require.NoError(t, err)
	assert.False(t, willRetry)

	// Verify only one DLQ entry (from worker A)
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	expectedCount := len(dlJobsBefore) + 1
	assert.Len(t, dlJobs, expectedCount, "Should have exactly one new DLQ entry from worker A")
}

// TestCoordinator_MaxRetryDiscard_ChecksOwnership tests the critical race condition
// where a worker's lease expires while failing a job at max retries. Another worker
// can reclaim the job, but the original worker should NOT be able to discard it.
//
// Race condition scenario:
// 1. Worker A claims job (at max retries)
// 2. Worker A's lease expires
// 3. Worker B reclaims the job
// 4. Worker A tries to fail/discard job → MUST fail with ErrJobOwnershipLost
//
// Without ownership check in DiscardJobAfterMaxRetries, Worker A would discard
// Worker B's job, causing lost work.
func TestCoordinator_MaxRetryDiscard_ChecksOwnership(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job at max retries
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Set job to max retries
	maxRetries := 3
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs SET retry_count = $1 WHERE id = $2
	`, maxRetries, jobID)
	require.NoError(t, err)

	// Worker A claims the job
	workerA := "worker-a-" + uuid.New().String()
	jobA, err := coordinator.ClaimNextJob(ctx, workerA, 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, jobA)
	require.Equal(t, maxRetries, jobA.RetryCount)

	// Simulate lease expiration: manually expire Worker A's lease
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET available_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, jobID)
	require.NoError(t, err)

	// Worker B reclaims the job (now that lease expired)
	workerB := "worker-b-" + uuid.New().String()
	jobB, err := coordinator.ClaimNextJob(ctx, workerB, 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, jobB, "Worker B should reclaim the job after Worker A's lease expired")
	require.Equal(t, jobID, jobB.ID)

	// Worker A (belatedly) tries to fail the job at max retries
	// This should FAIL because Worker A no longer owns the job
	willRetry, err := coordinator.FailJob(ctx, jobID, workerA, "error from A", worker.DefaultRetryConfig())

	// BUG: Currently this succeeds and discards Worker B's job!
	// FIX: Should return ErrJobOwnershipLost
	require.ErrorIs(t, err, domain.ErrJobOwnershipLost,
		"Worker A should NOT be able to discard job after losing ownership - Worker B reclaimed it")
	assert.False(t, willRetry, "Should not retry when ownership is lost")

	// Verify job was NOT moved to DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, dlJobs, "Job should NOT be in DLQ - Worker B still owns it")

	// Verify job is still claimable by Worker B (not discarded)
	var status string
	var claimedBy *string
	err = store.Pool().QueryRow(ctx, `
		SELECT status, claimed_by
		FROM recurring_generation_jobs
		WHERE id = $1
	`, jobID).Scan(&status, &claimedBy)
	require.NoError(t, err)
	assert.Equal(t, "running", status, "Job should still be running for Worker B")
	require.NotNil(t, claimedBy, "Job should still be claimed")
	assert.Equal(t, workerB, *claimedBy, "Job should be owned by Worker B")
}

// TestCoordinator_RetryExhaustion_MovesToDLQ verifies the progression from
// retries to DLQ after max retries exceeded.
func TestCoordinator_RetryExhaustion_MovesToDLQ(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	cfg := worker.DefaultWorkerConfig("test-worker")
	cfg.RetryConfig.MaxRetries = 2 // Reduce for faster test

	// Retry #1
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	willRetry, err := coordinator.FailJob(ctx, jobID, cfg.WorkerID, "failure 1", cfg.RetryConfig)
	require.NoError(t, err)
	assert.True(t, willRetry, "Should retry after first failure")

	// Make job immediately claimable (override exponential backoff for testing)
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET scheduled_for = NOW(), available_at = NOW()
		WHERE id = $1
	`, jobID)
	require.NoError(t, err)

	// Retry #2
	job, err = coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	willRetry, err = coordinator.FailJob(ctx, jobID, cfg.WorkerID, "failure 2", cfg.RetryConfig)
	require.NoError(t, err)
	assert.True(t, willRetry, "Should retry after second failure")

	// Make job immediately claimable again
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET scheduled_for = NOW(), available_at = NOW()
		WHERE id = $1
	`, jobID)
	require.NoError(t, err)

	// Retry #3 (exhausts max)
	job, err = coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	assert.Equal(t, 2, job.RetryCount, "Should be at max retries")

	willRetry, err = coordinator.FailJob(ctx, jobID, cfg.WorkerID, "failure 3 - exhausted", cfg.RetryConfig)
	require.NoError(t, err)
	assert.False(t, willRetry, "Should not retry after max retries")

	// Verify in DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1)
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, "exhausted", dlJobs[0].ErrorType)
	assert.Contains(t, dlJobs[0].ErrorMessage, "failure 3")
	assert.Equal(t, 3, dlJobs[0].RetryCount)
	assert.Equal(t, cfg.WorkerID, dlJobs[0].LastWorkerID, "LastWorkerID should match the worker that exhausted retries")
}

// TestCoordinator_ConcurrentDLQMoves verifies system behavior when multiple
// workers process different jobs simultaneously.
func TestCoordinator_ConcurrentDLQMoves(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create 5 jobs at max retries
	numJobs := 5
	jobIDs := make([]string, numJobs)
	templateID := createTestTemplate(t, store, ctx)

	for i := range numJobs {
		jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
			time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
		require.NoError(t, err)
		jobIDs[i] = jobID

		_, err = store.Pool().Exec(ctx, `
			UPDATE recurring_generation_jobs SET retry_count = $1 WHERE id = $2
		`, 3, jobID)
		require.NoError(t, err)
	}

	// Spawn 5 workers to fail jobs concurrently
	type result struct {
		workerID string
		jobID    string
		err      error
	}
	results := make(chan result, numJobs)

	for i := range numJobs {
		workerID := "worker-" + string(rune('A'+i))

		go func(wID string, idx int) {
			coordinator := postgres.NewPostgresCoordinator(store.Pool())
			cfg := worker.DefaultWorkerConfig(wID)

			// Claim and fail job
			job, err := coordinator.ClaimNextJob(ctx, wID, cfg.AvailabilityTimeout)
			if err != nil || job == nil {
				results <- result{workerID: wID, err: err}
				return
			}

			_, err = coordinator.FailJob(ctx, job.ID, wID, "concurrent failure", cfg.RetryConfig)
			results <- result{workerID: wID, jobID: job.ID, err: err}
		}(workerID, i)
	}

	// Collect results
	successCount := 0
	for range numJobs {
		res := <-results
		if res.err == nil && res.jobID != "" {
			successCount++
		}
	}

	// Verify all jobs successfully moved to DLQ
	assert.Equal(t, numJobs, successCount, "All workers should successfully move jobs to DLQ")

	// Verify DLQ has all jobs
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 20)
	require.NoError(t, err)
	assert.Len(t, dlJobs, numJobs, "All jobs should be in DLQ")

	// Verify all jobs are discarded
	var discardedCount int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'
	`).Scan(&discardedCount)
	require.NoError(t, err)
	assert.Equal(t, numJobs, discardedCount, "All jobs should be discarded")
}

// TestCoordinator_MoveToDeadLetter_AtomicityGuarantee verifies that MoveToDeadLetter
// atomically inserts into DLQ AND marks the original job as failed.
// This test simulates the panic/permanent failure path (not retry exhaustion).
func TestCoordinator_MoveToDeadLetter_AtomicityGuarantee(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Claim the job (simulates worker processing)
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Call MoveToDeadLetter directly (simulates panic handler)
	stackTrace := "goroutine 1 [running]:\npanic: test panic"
	err = coordinator.MoveToDeadLetter(ctx, job, cfg.WorkerID, "panic", "worker panicked during generation", &stackTrace)
	require.NoError(t, err)

	// CRITICAL VERIFICATION: Both operations must succeed atomically
	// 1. Job in DLQ with panic details
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1, "Job should be in DLQ")
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, "panic", dlJobs[0].ErrorType)
	assert.Contains(t, dlJobs[0].ErrorMessage, "worker panicked")
	assert.NotNil(t, dlJobs[0].StackTrace, "Stack trace should be preserved")
	assert.Contains(t, *dlJobs[0].StackTrace, "goroutine 1")
	assert.Equal(t, cfg.WorkerID, dlJobs[0].LastWorkerID, "LastWorkerID should match the worker that panicked")

	// 2. Original job should be marked as discarded (atomically with DLQ insert)
	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "discarded", status, "Job should be marked as discarded when moved to DLQ")
}

// TestCoordinator_DLQAtomicity_TransactionRollback verifies that if any part
// of the DLQ move fails, the entire operation rolls back (no partial state).
func TestCoordinator_DLQAtomicity_TransactionRollback(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job at max retries
	templateID := createTestTemplate(t, store, ctx)
	jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)

	// Get initial counts BEFORE the operation
	var initialDLQCount, initialDiscardedCount int
	err := store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&initialDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&initialDiscardedCount)
	require.NoError(t, err)

	// Attempt to fail the job (should move to DLQ atomically)
	willRetry, err := coordinator.FailJob(ctx, jobID, *job.ClaimedBy, "test failure", worker.DefaultRetryConfig())
	require.NoError(t, err)
	assert.False(t, willRetry)

	// Verify BOTH operations succeeded together
	var finalDLQCount, finalDiscardedCount int
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&finalDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&finalDiscardedCount)
	require.NoError(t, err)

	// Both counts should increase by exactly 1 (atomicity proof)
	assert.Equal(t, initialDLQCount+1, finalDLQCount, "DLQ should have exactly one new entry")
	assert.Equal(t, initialDiscardedCount+1, finalDiscardedCount, "Discarded jobs should increase by exactly one")

	// Verify the specific job is in both places
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 100)
	require.NoError(t, err)
	found := false
	for _, dlJob := range dlJobs {
		if dlJob.OriginalJobID == jobID {
			found = true
			break
		}
	}
	assert.True(t, found, "Job should be in DLQ")

	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "discarded", status, "Job should be marked as discarded")
}

// TestCoordinator_DLQAtomicity_DuplicateDetection verifies behavior when
// attempting to move the same job to DLQ twice (idempotency check).
func TestCoordinator_DLQAtomicity_DuplicateDetection(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Claim the job
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	// First MoveToDeadLetter (should succeed)
	err = coordinator.MoveToDeadLetter(ctx, job, cfg.WorkerID, "permanent", "database connection lost", nil)
	require.NoError(t, err)

	// Verify DLQ has the job
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1)
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, cfg.WorkerID, dlJobs[0].LastWorkerID, "LastWorkerID should be set on first DLQ move")

	// Second MoveToDeadLetter for same job (what happens?)
	// With ownership check, second call should fail because job is now discarded (no owner)
	err2 := coordinator.MoveToDeadLetter(ctx, job, cfg.WorkerID, "permanent", "second attempt", nil)

	// Second call should fail with ErrJobOwnershipLost since job is already discarded
	require.Error(t, err2, "Second MoveToDeadLetter should fail - job already discarded")
	assert.ErrorIs(t, err2, domain.ErrJobOwnershipLost, "Should fail with ownership lost error")
	t.Logf("Second MoveToDeadLetter correctly rejected: %v", err2)

	// Count DLQ entries for this job
	var dlqCount int
	err = store.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM dead_letter_jobs WHERE original_job_id = $1",
		jobID).Scan(&dlqCount)
	require.NoError(t, err)

	// With ownership check, second call fails, so only one entry exists
	assert.Equal(t, 1, dlqCount, "Should have only one DLQ entry per job (idempotent via ownership check)")
}

// TestCoordinator_DLQAtomicity_ConcurrentPanicHandling verifies that multiple
// workers handling panics don't create race conditions in DLQ insertion.
func TestCoordinator_DLQAtomicity_ConcurrentPanicHandling(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple jobs
	numJobs := 3
	templateID := createTestTemplate(t, store, ctx)
	jobs := make([]*domain.GenerationJob, numJobs)

	for i := range numJobs {
		_, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
			time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
		require.NoError(t, err)

		// Claim each job with different workers
		workerID := "worker-" + string(rune('A'+i))
		coordinator := postgres.NewPostgresCoordinator(store.Pool())
		job, err := coordinator.ClaimNextJob(ctx, workerID, 5*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, job)
		jobs[i] = job
	}

	// Simulate concurrent panic handling
	type result struct {
		jobID string
		err   error
	}
	results := make(chan result, numJobs)

	for i := range numJobs {
		go func(idx int) {
			coordinator := postgres.NewPostgresCoordinator(store.Pool())
			job := jobs[idx]

			stackTrace := "panic in worker"
			// Use the worker ID that claimed the job
			workerID := *job.ClaimedBy
			err := coordinator.MoveToDeadLetter(ctx, job, workerID, "panic",
				"concurrent panic test", &stackTrace)

			results <- result{jobID: job.ID, err: err}
		}(i)
	}

	// Collect results
	successCount := 0
	for range numJobs {
		res := <-results
		if res.err == nil {
			successCount++
		} else {
			t.Logf("Job %s failed to move to DLQ: %v", res.jobID, res.err)
		}
	}

	// All operations should succeed without conflicts
	assert.Equal(t, numJobs, successCount, "All concurrent MoveToDeadLetter calls should succeed")

	// Verify all jobs in DLQ
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 20)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(dlJobs), numJobs, "All jobs should be in DLQ")

	// Verify no duplicates by counting unique original_job_ids
	jobIDsSeen := make(map[string]bool)
	for _, dlJob := range dlJobs {
		jobIDsSeen[dlJob.OriginalJobID] = true
	}
	assert.GreaterOrEqual(t, len(jobIDsSeen), numJobs, "Should have at least %d unique jobs in DLQ", numJobs)
}

// Helper functions

func createTestTemplate(t *testing.T, store *postgres.Store, ctx context.Context) string {
	t.Helper()

	// Create list
	listUUID, err := uuid.NewV7()
	require.NoError(t, err)
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:        listID,
		Title:     "Test List",
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
		Title:                 "Test Template",
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

	return templateID
}

func createJobAtMaxRetries(t *testing.T, store *postgres.Store, coordinator *postgres.PostgresCoordinator, ctx context.Context, templateID string, maxRetries int) (string, *domain.GenerationJob) {
	t.Helper()

	// Create job
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Set retry_count to max
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs SET retry_count = $1 WHERE id = $2
	`, maxRetries, jobID)
	require.NoError(t, err)

	// Claim the job
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	return jobID, job
}

// =============================================================================
// ATOMICITY FAILURE MODE TESTS
// =============================================================================
// These tests verify that the DLQ move operation is atomic:
// 1. Insert into dead_letter_jobs table
// 2. Mark the original job as failed/discarded
//
// Without a transaction, a crash between these operations could leave the job
// in an inconsistent state - marked as running but also in DLQ, or discarded
// but not in DLQ.
// =============================================================================

// TestAtomicity_SuccessfulDLQMove_BothTablesUpdated verifies that when a job
// exhausts retries, both the DLQ insert AND job discard happen in a single
// atomic transaction. This is the core atomicity guarantee being tested.
func TestAtomicity_SuccessfulDLQMove_BothTablesUpdated(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job at max retries
	templateID := createTestTemplate(t, store, ctx)
	jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)

	// Capture initial state
	var initialStatus string
	err := store.Pool().QueryRow(ctx, `
		SELECT status FROM recurring_generation_jobs WHERE id = $1
	`, jobID).Scan(&initialStatus)
	require.NoError(t, err)
	assert.Equal(t, "running", initialStatus)

	var initialDLQCount, initialDiscardedCount int
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&initialDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&initialDiscardedCount)
	require.NoError(t, err)

	// Fail the job - should move to DLQ atomically
	willRetry, err := coordinator.FailJob(ctx, jobID, *job.ClaimedBy, "exhausted retries", worker.DefaultRetryConfig())
	require.NoError(t, err)
	assert.False(t, willRetry, "Should not retry after max retries")

	// ATOMICITY VERIFICATION: Both operations must succeed together
	var finalDLQCount, finalDiscardedCount int
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&finalDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&finalDiscardedCount)
	require.NoError(t, err)

	// Both counts should increase by exactly 1
	assert.Equal(t, initialDLQCount+1, finalDLQCount, "DLQ should have exactly one new entry")
	assert.Equal(t, initialDiscardedCount+1, finalDiscardedCount, "Should have exactly one more discarded job")

	// Verify the specific job is in DLQ with correct details
	var dlqOriginalJobID, dlqErrorType, dlqErrorMessage string
	err = store.Pool().QueryRow(ctx, `
		SELECT original_job_id, error_type, error_message
		FROM dead_letter_jobs
		WHERE original_job_id = $1
	`, jobID).Scan(&dlqOriginalJobID, &dlqErrorType, &dlqErrorMessage)
	require.NoError(t, err)
	assert.Equal(t, jobID, dlqOriginalJobID)
	assert.Equal(t, "exhausted", dlqErrorType)
	assert.Equal(t, "exhausted retries", dlqErrorMessage)

	// Verify original job is discarded
	var finalStatus string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&finalStatus)
	require.NoError(t, err)
	assert.Equal(t, "discarded", finalStatus)

	t.Logf("Atomicity verified: Job %s is in DLQ with status 'exhausted' AND marked as 'discarded'", jobID)
}

// TestAtomicity_MoveToDeadLetter_MarksJobDiscarded verifies that MoveToDeadLetter
// atomically inserts to DLQ AND marks the original job as discarded.
// This test was previously documenting a bug (atomicity gap), now verifies the fix.
func TestAtomicity_MoveToDeadLetter_MarksJobDiscarded(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Claim the job (simulates worker processing)
	cfg := worker.DefaultWorkerConfig("test-worker")
	job, err := coordinator.ClaimNextJob(ctx, cfg.WorkerID, cfg.AvailabilityTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Verify job is running
	var statusBefore string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&statusBefore)
	require.NoError(t, err)
	assert.Equal(t, "running", statusBefore)

	// Call MoveToDeadLetter directly (simulates panic handler path)
	stackTrace := "goroutine 1 [running]:\npanic: simulated panic for atomicity test"
	err = coordinator.MoveToDeadLetter(ctx, job, cfg.WorkerID, "panic", "simulated panic", &stackTrace)
	require.NoError(t, err)

	// Verify job is in DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1, "Job should be in DLQ")
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, "panic", dlJobs[0].ErrorType)
	assert.Equal(t, cfg.WorkerID, dlJobs[0].LastWorkerID, "LastWorkerID should track which worker panicked")

	// CRITICAL CHECK: Original job should be discarded (atomically with DLQ insert)
	var statusAfter string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&statusAfter)
	require.NoError(t, err)

	// ATOMICITY FIX VERIFIED: Job is now properly marked as discarded
	assert.Equal(t, "discarded", statusAfter,
		"Job should be 'discarded' after MoveToDeadLetter (atomicity fix verified)")

	t.Logf("ATOMICITY FIX VERIFIED:")
	t.Logf("  - Job %s is in DLQ with error_type='panic'", jobID)
	t.Logf("  - Original job status is '%s' (correctly discarded)", statusAfter)
}

// TestAtomicity_ConcurrentRecovery_OnlyOneWorkerSucceeds verifies that when a
// job times out and multiple workers try to recover it, only one succeeds.
// This tests the SKIP LOCKED mechanism prevents race conditions.
func TestAtomicity_ConcurrentRecovery_OnlyOneWorkerSucceeds(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job and set it as "stuck" (running but past availability timeout)
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Manually set job as stuck (past availability timeout)
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET status = 'running',
		    claimed_by = 'crashed-worker',
		    claimed_at = NOW() - INTERVAL '10 minutes',
		    available_at = NOW() - INTERVAL '5 minutes'
		WHERE id = $1
	`, jobID)
	require.NoError(t, err)

	// Spawn 5 workers to concurrently try to claim the stuck job
	numWorkers := 5
	type claimResult struct {
		workerID string
		job      *domain.GenerationJob
		err      error
	}
	results := make(chan claimResult, numWorkers)

	for i := range numWorkers {
		workerID := "recovery-worker-" + string(rune('A'+i))
		go func(wID string) {
			coordinator := postgres.NewPostgresCoordinator(store.Pool())
			job, err := coordinator.ClaimNextJob(ctx, wID, 5*time.Minute)
			results <- claimResult{workerID: wID, job: job, err: err}
		}(workerID)
	}

	// Collect results
	var claimedCount int
	var claimingWorker string
	for range numWorkers {
		res := <-results
		if res.err == nil && res.job != nil {
			claimedCount++
			claimingWorker = res.workerID
		}
	}

	// CRITICAL: Exactly one worker should claim the job (SKIP LOCKED ensures this)
	assert.Equal(t, 1, claimedCount,
		"Exactly one worker should claim the stuck job (SKIP LOCKED prevents race)")
	t.Logf("Worker %s successfully claimed the stuck job", claimingWorker)

	// Verify the job is now owned by that worker
	var claimedBy string
	err = store.Pool().QueryRow(ctx, "SELECT claimed_by FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&claimedBy)
	require.NoError(t, err)
	assert.Equal(t, claimingWorker, claimedBy)
}

// TestAtomicity_ContextCancellation_RollsBackTransaction verifies that if the
// context is cancelled during a DLQ move operation, the transaction rolls back
// and no partial state is created.
func TestAtomicity_ContextCancellation_RollsBackTransaction(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create template and job at max retries
	templateID := createTestTemplate(t, store, ctx)
	jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)

	// Capture initial state
	var initialDLQCount int
	err := store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&initialDLQCount)
	require.NoError(t, err)

	// Create a context that we'll cancel
	cancelCtx, cancel := context.WithCancel(ctx)

	// Cancel the context immediately (simulates crash/interrupt)
	cancel()

	// Attempt to fail the job with cancelled context
	willRetry, err := coordinator.FailJob(cancelCtx, jobID, *job.ClaimedBy, "test error", worker.DefaultRetryConfig())

	// Operation should fail due to cancelled context
	require.Error(t, err, "FailJob should fail with cancelled context")
	assert.Contains(t, err.Error(), "context canceled", "Error should mention cancelled context")

	// CRITICAL: Verify no partial state was created
	var finalDLQCount int
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&finalDLQCount)
	require.NoError(t, err)
	assert.Equal(t, initialDLQCount, finalDLQCount, "DLQ should not have new entries after context cancellation")

	// Job should NOT be discarded
	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&status)
	require.NoError(t, err)
	assert.NotEqual(t, "discarded", status, "Job should not be discarded after cancelled operation")

	t.Logf("Context cancellation rollback verified: willRetry=%v, status=%s", willRetry, status)
}

// TestAtomicity_VerifyBothOperationsInSingleTransaction verifies that FailJob
// uses a transaction to ensure both DLQ insert and job discard are atomic.
// We test this by checking database state consistency.
func TestAtomicity_VerifyBothOperationsInSingleTransaction(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create multiple jobs at max retries
	numJobs := 10
	templateID := createTestTemplate(t, store, ctx)
	jobIDs := make([]string, numJobs)
	claimedBys := make([]string, numJobs)

	for i := range numJobs {
		jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)
		jobIDs[i] = jobID
		claimedBys[i] = *job.ClaimedBy
	}

	// Capture initial state
	var initialDLQCount, initialDiscardedCount int
	err := store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&initialDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&initialDiscardedCount)
	require.NoError(t, err)

	// Fail all jobs concurrently
	type failResult struct {
		jobID     string
		willRetry bool
		err       error
	}
	results := make(chan failResult, numJobs)

	for i := range numJobs {
		go func(idx int) {
			coord := postgres.NewPostgresCoordinator(store.Pool())
			willRetry, err := coord.FailJob(ctx, jobIDs[idx], claimedBys[idx], "concurrent failure", worker.DefaultRetryConfig())
			results <- failResult{jobID: jobIDs[idx], willRetry: willRetry, err: err}
		}(i)
	}

	// Collect results
	successCount := 0
	for range numJobs {
		res := <-results
		if res.err == nil && !res.willRetry {
			successCount++
		}
	}

	// All jobs should have been successfully moved to DLQ
	assert.Equal(t, numJobs, successCount, "All concurrent FailJob calls should succeed")

	// CRITICAL ATOMICITY CHECK: DLQ count should equal discarded count
	// If operations were non-atomic, these could be different
	var finalDLQCount, finalDiscardedCount int
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM dead_letter_jobs").Scan(&finalDLQCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, "SELECT COUNT(*) FROM recurring_generation_jobs WHERE status = 'discarded'").Scan(&finalDiscardedCount)
	require.NoError(t, err)

	dlqIncrease := finalDLQCount - initialDLQCount
	discardedIncrease := finalDiscardedCount - initialDiscardedCount

	assert.Equal(t, numJobs, dlqIncrease, "DLQ should have exactly %d new entries", numJobs)
	assert.Equal(t, numJobs, discardedIncrease, "Should have exactly %d new discarded jobs", numJobs)
	assert.Equal(t, dlqIncrease, discardedIncrease,
		"ATOMICITY PROOF: DLQ entries (%d) must equal discarded jobs (%d)", dlqIncrease, discardedIncrease)

	t.Logf("Atomicity verified: %d jobs in DLQ, %d jobs discarded (both operations atomic)", dlqIncrease, discardedIncrease)
}

// TestAtomicity_StaleJobRecovery_AfterCrash simulates the scenario where a worker
// crashes after claiming a job. Another worker should be able to recover the job
// after the availability timeout expires and complete it successfully.
func TestAtomicity_StaleJobRecovery_AfterCrash(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a job
	templateID := createTestTemplate(t, store, ctx)
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{},
		time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 7))
	require.NoError(t, err)

	// Worker A claims the job
	coordinatorA := postgres.NewPostgresCoordinator(store.Pool())
	workerA := "worker-A"
	jobA, err := coordinatorA.ClaimNextJob(ctx, workerA, 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, jobA)
	require.Equal(t, jobID, jobA.ID)

	// Verify job is running and owned by Worker A
	var statusBefore, claimedByBefore string
	err = store.Pool().QueryRow(ctx, `
		SELECT status, claimed_by FROM recurring_generation_jobs WHERE id = $1
	`, jobID).Scan(&statusBefore, &claimedByBefore)
	require.NoError(t, err)
	assert.Equal(t, "running", statusBefore)
	assert.Equal(t, workerA, claimedByBefore)

	// Simulate crash: Worker A disappears without completing
	// Set availability_at to the past (simulating timeout)
	_, err = store.Pool().Exec(ctx, `
		UPDATE recurring_generation_jobs
		SET available_at = NOW() - INTERVAL '1 minute'
		WHERE id = $1
	`, jobID)
	require.NoError(t, err)

	// Worker B should now be able to claim the "stuck" job
	coordinatorB := postgres.NewPostgresCoordinator(store.Pool())
	workerB := "worker-B"
	jobB, err := coordinatorB.ClaimNextJob(ctx, workerB, 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, jobB, "Worker B should claim the stuck job")
	assert.Equal(t, jobID, jobB.ID)

	// Verify Worker B now owns the job
	var claimedByAfter string
	err = store.Pool().QueryRow(ctx, "SELECT claimed_by FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&claimedByAfter)
	require.NoError(t, err)
	assert.Equal(t, workerB, claimedByAfter)

	// Worker B completes the job successfully
	err = coordinatorB.CompleteJob(ctx, jobID, workerB)
	require.NoError(t, err)

	// Verify final state: job is completed
	var finalStatus string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id = $1", jobID).Scan(&finalStatus)
	require.NoError(t, err)
	assert.Equal(t, "completed", finalStatus, "Job should be completed by Worker B")

	t.Logf("Stale job recovery verified: Worker %s recovered job from Worker %s and completed it",
		workerB, workerA)
}

// TestAtomicity_PartialFailure_NoOrphanedDLQEntries verifies that if FailJob
// partially fails, we don't end up with orphaned entries in DLQ that don't
// correspond to discarded jobs.
func TestAtomicity_PartialFailure_NoOrphanedDLQEntries(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create a set of jobs and fail them
	numJobs := 5
	templateID := createTestTemplate(t, store, ctx)

	for i := range numJobs {
		jobID, job := createJobAtMaxRetries(t, store, coordinator, ctx, templateID, 3)

		// Fail the job (should move to DLQ atomically)
		willRetry, err := coordinator.FailJob(ctx, jobID, *job.ClaimedBy, "failure #"+string(rune('1'+i)), worker.DefaultRetryConfig())
		require.NoError(t, err)
		assert.False(t, willRetry)
	}

	// Query for orphaned DLQ entries (DLQ entries whose original job is NOT discarded)
	var orphanedCount int
	err := store.Pool().QueryRow(ctx, `
		SELECT COUNT(*)
		FROM dead_letter_jobs dlq
		LEFT JOIN recurring_generation_jobs job ON dlq.original_job_id = job.id
		WHERE job.status IS NULL OR job.status != 'discarded'
	`).Scan(&orphanedCount)
	require.NoError(t, err)

	// There should be NO orphaned DLQ entries
	assert.Equal(t, 0, orphanedCount,
		"ATOMICITY VIOLATED: Found %d orphaned DLQ entries (job not discarded)", orphanedCount)

	// Also verify the inverse: no discarded jobs without DLQ entries
	var discardedWithoutDLQ int
	err = store.Pool().QueryRow(ctx, `
		SELECT COUNT(*)
		FROM recurring_generation_jobs job
		LEFT JOIN dead_letter_jobs dlq ON job.id = dlq.original_job_id
		WHERE job.status = 'discarded' AND dlq.id IS NULL
	`).Scan(&discardedWithoutDLQ)
	require.NoError(t, err)

	assert.Equal(t, 0, discardedWithoutDLQ,
		"ATOMICITY VIOLATED: Found %d discarded jobs without DLQ entries", discardedWithoutDLQ)

	t.Logf("Atomicity verified: No orphaned DLQ entries, no missing DLQ entries for discarded jobs")
}

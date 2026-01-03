package integration

import (
	"context"
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

// TestWorker_Lifecycle_SkipLockedExclusivity verifies that multiple workers
// running concurrently never claim the same job, ensuring strict exclusivity.
func TestWorker_Lifecycle_SkipLockedExclusivity(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a single job
	listUUID, _ := uuid.NewV7()
	listID := listUUID.String()
	store.CreateList(ctx, &domain.TodoList{ID: listID, Title: "Test List"})

	templateUUID, _ := uuid.NewV7()
	templateID := templateUUID.String()
	template := &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listID,
		Title:             "Test Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	}
	store.CreateRecurringTemplate(ctx, template)

	// Schedule one job
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{}, time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))
	require.NoError(t, err)

	// Launch multiple workers trying to claim the SAME job
	// We use the Coordinator directly to isolate the claim logic
	numWorkers := 10
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	var claimedCount int32
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerID := "worker-" + uuid.New().String()
		go func(wID string) {
			defer wg.Done()

			// Try to claim
			job, err := coordinator.ClaimNextJob(ctx, wID, 1*time.Minute)
			if err != nil {
				return // Error or no job found
			}
			if job != nil {
				if job.ID == jobID {
					atomic.AddInt32(&claimedCount, 1)
					// Hold it briefly to ensure overlap
					time.Sleep(100 * time.Millisecond)
				}
			}
		}(workerID)
	}

	wg.Wait()

	// CRITICAL: Exactly ONE worker should have succeeded
	assert.Equal(t, int32(1), claimedCount, "Only exactly one worker should be able to claim the job")
}

// TestWorker_Lifecycle_HeartbeatExtension verifies that a worker extends
// the visibility timeout while processing a long-running job.
// Note: This requires the worker implementation to support heartbeating,
// which is typically handled by the Coordinator or the Worker loop.
// Since we are testing the Coordinator/Worker interaction, we will simulate
// the heartbeat updates manually or via the Coordinator's ExtendJobLock method.
func TestWorker_Lifecycle_HeartbeatExtension(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Create job
	listUUID, _ := uuid.NewV7()
	store.CreateList(ctx, &domain.TodoList{ID: listUUID.String(), Title: "List"})

	templateUUID, _ := uuid.NewV7()
	templateID := templateUUID.String()
	store.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listUUID.String(),
		Title:             "Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	})

	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{}, time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))
	require.NoError(t, err)

	// Claim job
	workerID := "worker-heartbeat"
	initialTimeout := 2 * time.Second
	job, err := coordinator.ClaimNextJob(ctx, workerID, initialTimeout)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Record initial available_at
	var initialAvailableAt time.Time
	err = store.Pool().QueryRow(ctx, "SELECT available_at FROM recurring_generation_jobs WHERE id=$1", jobID).Scan(&initialAvailableAt)
	require.NoError(t, err)

	// Extend lock
	extension := 5 * time.Second
	err = coordinator.ExtendAvailability(ctx, jobID, workerID, extension)
	require.NoError(t, err)

	// Verify available_at moved forward
	var newAvailableAt time.Time
	err = store.Pool().QueryRow(ctx, "SELECT available_at FROM recurring_generation_jobs WHERE id=$1", jobID).Scan(&newAvailableAt)
	require.NoError(t, err)

	assert.True(t, newAvailableAt.After(initialAvailableAt), "AvailableAt should be extended forward")

	// CRITICAL: Compare DB timestamps only to avoid clock skew
	// Calculate actual extension: newAvailableAt (DB) - initialAvailableAt (DB)
	actualExtension := newAvailableAt.Sub(initialAvailableAt)

	// Verify extension is approximately the requested duration (allow 2s tolerance for execution time)
	// This avoids comparing Go's time.Now() with PostgreSQL's NOW()
	assert.InDelta(t, extension.Seconds(), actualExtension.Seconds(), 2.0,
		"Extension should be approximately %v (got %v)", extension, actualExtension)
}

// TestWorker_Lifecycle_CrashRecovery verifies that if a worker "crashes"
// (stops heartbeating), the job becomes available for another worker.
func TestWorker_Lifecycle_CrashRecovery(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Setup job
	listUUID, _ := uuid.NewV7()
	store.CreateList(ctx, &domain.TodoList{ID: listUUID.String(), Title: "List"})

	templateUUID, _ := uuid.NewV7()
	templateID := templateUUID.String()
	store.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listUUID.String(),
		Title:             "Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	})

	jobID, _ := store.ScheduleGenerationJob(ctx, templateID, time.Time{}, time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))

	// 1. Worker A claims the job with short timeout
	workerA := "worker-A"
	shortTimeout := 1 * time.Second
	jobA, err := coordinator.ClaimNextJob(ctx, workerA, shortTimeout)
	require.NoError(t, err)
	require.NotNil(t, jobA)
	assert.Equal(t, jobID, jobA.ID)

	// 2. Worker A "crashes" (we do nothing and wait for timeout to expire)
	time.Sleep(shortTimeout + 100*time.Millisecond)

	// 3. Worker B tries to claim
	workerB := "worker-B"
	jobB, err := coordinator.ClaimNextJob(ctx, workerB, 1*time.Minute)
	require.NoError(t, err)

	// Should successfully claim the SAME job
	require.NotNil(t, jobB, "Worker B should be able to claim the expired job")
	assert.Equal(t, jobID, jobB.ID)

	// Verify ownership change in DB
	var claimedBy string
	err = store.Pool().QueryRow(ctx, "SELECT claimed_by FROM recurring_generation_jobs WHERE id=$1", jobID).Scan(&claimedBy)
	require.NoError(t, err)
	assert.Equal(t, workerB, claimedBy)
}

// TestWorker_Lifecycle_ReconciliationExclusivity ensures that the exclusive locking
// mechanism (TryAcquireExclusiveRun) prevents concurrent reconciliation runs.
// Only one worker should acquire the lease; others should be blocked.
func TestWorker_Lifecycle_ReconciliationExclusivity(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	coordinator := postgres.NewPostgresCoordinator(store.Pool())

	// Run TryAcquireExclusiveRun concurrently - only one should succeed
	var wg sync.WaitGroup
	var acquiredCount atomic.Int32
	concurrency := 5
	leaseDuration := 5 * time.Second

	for range concurrency {
		workerID := "reconciler-" + uuid.New().String()
		wg.Go(func() {
			release, acquired, err := coordinator.TryAcquireExclusiveRun(
				ctx,
				worker.ReconciliationRunType,
				workerID,
				leaseDuration,
			)
			if err != nil {
				t.Logf("Worker %s failed to acquire: %v", workerID, err)
				return
			}
			if acquired {
				acquiredCount.Add(1)
				// Hold the lease briefly to ensure others can't acquire
				time.Sleep(50 * time.Millisecond)
				release()
			}
		})
	}
	wg.Wait()

	// Verify only ONE worker acquired the exclusive lease
	assert.Equal(t, int32(1), acquiredCount.Load(), "Only one worker should acquire exclusive lease")
}

// TestWorker_Lifecycle_PanicHandling_MovesToDLQ verifies that if the worker
// panics during processing, the recovery mechanism catches it and moves the
// job to the DLQ.
func TestWorker_Lifecycle_PanicHandling_MovesToDLQ(t *testing.T) {
	store, _, cleanup := setupWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Setup template
	listUUID, _ := uuid.NewV7()
	store.CreateList(ctx, &domain.TodoList{ID: listUUID.String(), Title: "List"})

	templateUUID, _ := uuid.NewV7()
	templateID := templateUUID.String()
	store.CreateRecurringTemplate(ctx, &domain.RecurringTemplate{
		ID:                templateID,
		ListID:            listUUID.String(),
		Title:             "Task",
		RecurrencePattern: domain.RecurrenceDaily,
		RecurrenceConfig:  map[string]any{"interval": float64(1)},
		IsActive:          true,
	})

	// Schedule job
	jobID, err := store.ScheduleGenerationJob(ctx, templateID, time.Time{}, time.Now().UTC(), time.Now().UTC().Add(24*time.Hour))
	require.NoError(t, err)

	// Create worker with faulty repo that panics
	faulty := &faultyRepo{
		Repository: store,
		panicWith:  "simulated panic",
	}

	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	generator := recurring.NewDomainGenerator()
	cfg := worker.DefaultWorkerConfig("panic-worker")

	genWorker := worker.NewGenerationWorker(coordinator, faulty, generator, cfg)

	// Process (should catch panic)
	err = genWorker.RunProcessOnce(ctx)
	require.NoError(t, err, "RunProcessOnce should swallow panics and return nil")

	// Verify job is in DLQ
	dlJobs, err := coordinator.ListDeadLetterJobs(ctx, 10)
	require.NoError(t, err)
	require.Len(t, dlJobs, 1)
	assert.Equal(t, jobID, dlJobs[0].OriginalJobID)
	assert.Equal(t, "panic", dlJobs[0].ErrorType)
	assert.Contains(t, dlJobs[0].ErrorMessage, "simulated panic")

	// Verify job is discarded
	var status string
	err = store.Pool().QueryRow(ctx, "SELECT status FROM recurring_generation_jobs WHERE id=$1", jobID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "discarded", status)
}

// faultyRepo wraps the real repository to inject failures
type faultyRepo struct {
	worker.Repository
	failWith  error
	panicWith any
}

func (r *faultyRepo) ListExceptions(ctx context.Context, templateID string, from, until time.Time) ([]*domain.RecurringTemplateException, error) {
	if r.panicWith != nil {
		panic(r.panicWith)
	}
	if r.failWith != nil {
		return nil, r.failWith
	}
	return r.Repository.ListExceptions(ctx, templateID, from, until)
}

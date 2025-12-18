# Worker End-to-End Testing Design

**Date:** 2025-12-18
**Status:** Approved

## Overview

Design for comprehensive end-to-end testing of the worker system, including multi-worker scenarios, high-load testing, failure recovery, and complete flow validation with real PostgreSQL database.

## Goals

1. Test complete worker flow: template creation → job scheduling → job processing → task generation
2. Verify multiple workers can process jobs concurrently without conflicts
3. Test high-load scenarios with 500+ templates generating 6,500+ tasks
4. Validate failure recovery and edge cases
5. Ensure all recurrence patterns (daily, weekly, monthly, quarterly, yearly, weekdays) work correctly
6. Verify future task generation across different generation windows

## Design

### 1. Worker Package Refactoring

**Current State:** Worker logic is in `cmd/worker/main.go` with functions that are not reusable or testable.

**New Structure:** `internal/worker/worker.go`

```go
type Worker struct {
    store           core.Storage
    generator       *recurring.Generator
    scheduleInterval time.Duration
    processInterval  time.Duration
    done            chan struct{}
    wg              sync.WaitGroup
}

func New(store core.Storage, opts ...Option) *Worker
func (w *Worker) Start(ctx context.Context) error
func (w *Worker) Stop() error
func (w *Worker) RunScheduleOnce(ctx context.Context) error
func (w *Worker) RunProcessOnce(ctx context.Context) (jobProcessed bool, err error)
```

**Design Decisions:**
- **Configurable intervals** - Tests can use 0s intervals or control execution via `RunOnce` methods
- **Context-aware** - Respects cancellation for graceful shutdown
- **Testable units** - `RunScheduleOnce()` and `RunProcessOnce()` enable precise test control
- **Production-ready** - `Start()` runs full ticker loops for production use
- **Zero duplication** - `cmd/worker/main.go` becomes thin wrapper

### 2. Test Organization

**File:** `tests/integration/worker_test.go`

Tests placed in existing integration suite to reuse PostgreSQL test infrastructure.

**Test Categories:**

**Single Worker Tests (Foundational):**
- `TestWorker_CompleteFlow` - End-to-end flow with multiple recurrence patterns
- `TestWorker_ScheduleJobCreation` - Job creation from templates
- `TestWorker_ProcessJob` - Job processing and task generation
- `TestWorker_NoJobsAvailable` - Worker behavior when queue empty
- `TestWorker_JobFailureHandling` - Error handling and job status updates

**Multi-Worker Tests (Concurrency):**
- `TestWorker_MultipleWorkers_JobDistribution` - N workers, M jobs, verify distribution
- `TestWorker_MultipleWorkers_HighLoad` - 500+ templates, 10 workers, 6,500+ tasks
- `TestWorker_MultipleWorkers_SkipLocked` - Verify no double-claiming

**Edge Cases and Recovery:**
- `TestWorker_CrashedWorker_JobRecovery` - Stale RUNNING jobs become claimable
- `TestWorker_RetryLogic` - Failed jobs increment retry_count
- `TestWorker_GenerationWindow` - Future task generation and window advancement

### 3. Test Scenarios Detail

#### Complete Flow Test (Multi-Pattern)

Creates templates with diverse patterns and verifies correct future task generation:

- **Daily** (interval=1, window=30 days) → ~30 tasks
- **Weekly** (Monday, window=90 days) → ~12 tasks
- **Monthly** (day=15, window=365 days) → ~12 tasks
- **Quarterly** (window=730 days) → ~8 tasks
- **Weekdays** (window=30 days) → ~22 tasks

**Verifications:**
- Correct number of tasks for each pattern
- Tasks span entire generation window
- Task dates match pattern rules (e.g., monthly on 15th)
- No past tasks created

#### High Load Test

**Scale:**
- 500 templates total:
  - 200 daily (7-30 day windows) → ~3,000 tasks
  - 150 weekly (60-90 day windows) → ~2,000 tasks
  - 100 monthly (180-365 day windows) → ~1,000 tasks
  - 50 mixed patterns → ~500 tasks
- **Total:** ~6,500 tasks across various future dates
- 10 concurrent workers

**Verifications:**
- All 500 jobs completed
- All ~6,500 tasks created with correct dates
- Future tasks span months/years correctly
- Performance metrics (tasks/sec, completion time)
- No job processed twice

#### Generation Window Test

Tests progressive generation and deduplication:

1. Template: monthly, `last_generated_until=2025-06-01`, `generation_window=180 days`
2. Current date: 2025-12-18
3. First run: Creates tasks from 2025-06-01 to 2026-06-15
4. Verify: `last_generated_until` updated
5. Second run: No duplicates created
6. Advance time, run again: Only new future tasks created

#### Multi-Worker Job Distribution

**Setup:**
- 10 templates with 10 pending jobs
- 3 workers in goroutines
- Each worker runs `RunProcessOnce()` in loop

**Verifications:**
- All 10 jobs completed
- No job claimed twice (PostgreSQL SKIP LOCKED)
- Load balanced (~3-4 jobs per worker)
- Job status transitions tracked

#### Failure Recovery

**Scenario:**
1. Worker claims job (status=RUNNING)
2. Worker crashes (doesn't update job)
3. Job becomes stale (started_at > timeout threshold)
4. New worker should reclaim and complete

**Verifications:**
- Stale RUNNING jobs become claimable
- Job completed by recovery worker
- Error messages preserved
- Retry count incremented

### 4. Test Infrastructure

**Database Setup:**
- Reuse existing `setupTestDB()` from integration tests
- Each test gets fresh state
- Test database runs on port 5433 via docker-compose.test.yml

**Worker Coordination:**
- Single-worker tests: Use `RunOnce()` methods for deterministic control
- Multi-worker tests: Launch goroutines with `Start()`, coordinate via channels
- Use `sync.WaitGroup` and atomic counters for synchronization

**Test Utilities:**
```go
// Helper to create test templates with various patterns
func createTestTemplates(t *testing.T, store core.Storage, count int, pattern string) []uuid.UUID

// Helper to verify task generation
func verifyTasksGenerated(t *testing.T, store core.Storage, listID uuid.UUID, expectedCount int, dateRange time.Time)

// Helper to run multiple workers concurrently
func runWorkersUntilDone(ctx context.Context, workers []*worker.Worker, timeout time.Duration) error
```

## Implementation Plan

1. ✅ Design approved
2. Create `internal/worker/worker.go` with Worker struct
3. Extract logic from `cmd/worker/main.go` → Worker methods
4. Update `cmd/worker/main.go` to use new Worker type
5. Create `tests/integration/worker_test.go`
6. Implement foundational single-worker tests
7. Implement multi-worker concurrency tests
8. Implement high-load tests (500+ templates)
9. Implement failure recovery tests
10. Implement generation window tests
11. Run full test suite and verify

## Success Criteria

- All tests pass with real PostgreSQL database
- Zero code duplication between production and test code
- Multi-worker tests demonstrate SKIP LOCKED behavior
- High-load test completes successfully with 500+ templates
- All recurrence patterns tested with future task generation
- Failure recovery scenarios handled correctly
- Tests run in reasonable time (< 2 minutes for full suite)

## Non-Goals

- Testing with other databases (PostgreSQL only)
- Performance benchmarking (separate effort)
- Testing worker retry logic (not yet implemented)
- Testing job timeout handling (future enhancement)

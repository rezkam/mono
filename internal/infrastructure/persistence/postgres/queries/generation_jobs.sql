-- Generation Job Queue - Timestamp Fields Explained
-- ====================================================
-- scheduled_for: WHEN the job should execute (user's intent)
--   - Set at creation or retry scheduling
--   - Determines job ordering (earliest scheduled_for processed first)
--   - For pending jobs: must be <= NOW() to be claimable
--
-- available_at: WHEN the job can be claimed by workers (availability window)
--   - For pending jobs: equals scheduled_for (immediately available when scheduled)
--   - For running jobs: NOW() + timeout (e.g., 5 minutes visibility window)
--   - Enables stuck job recovery: if worker crashes, job becomes claimable when available_at <= NOW()
--   - Pattern inspired by SQS Visibility Timeout
--
-- Example timeline:
--   T+0s:  Job created with scheduled_for=NOW(), available_at=NOW()
--   T+1s:  Worker claims job → status='running', available_at=NOW()+5min
--   T+301s: Worker crashes (doesn't complete or extend)
--   T+301s: Job becomes claimable again (available_at <= NOW())

-- name: InsertGenerationJob :exec
-- Insert a single generation job
INSERT INTO recurring_generation_jobs (
    id, template_id, generate_from, generate_until,
    scheduled_for, status, retry_count, created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8
);

-- name: FindGenerationJobByID :one
-- Retrieve a generation job by ID
SELECT * FROM recurring_generation_jobs
WHERE id = $1;

-- name: ClaimNextPendingJob :one
-- Atomically claim the next pending or stuck running job using SKIP LOCKED.
-- Returns the job if claimed, or NULL if no jobs available.
-- This query covers two scenarios:
--   1. Pending jobs ready to run (scheduled_for <= NOW)
--   2. Running jobs past availability timeout (stuck workers)
SELECT id, template_id, generate_from, generate_until, retry_count, scheduled_for, created_at
FROM recurring_generation_jobs
WHERE (status = 'pending' AND scheduled_for <= NOW())
   OR (status = 'running' AND available_at <= NOW())
ORDER BY scheduled_for
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: MarkJobAsRunning :execrows
-- Mark a claimed job as running with worker ownership and availability timeout.
-- Returns 0 rows if job doesn't exist or was already claimed by another worker.
UPDATE recurring_generation_jobs
SET status = 'running',
    started_at = NOW(),
    claimed_by = $2,
    claimed_at = NOW(),
    available_at = $3
WHERE id = $1;

-- name: CompleteJobWithOwnershipCheck :execrows
-- Mark job as completed, but only if still owned by the specified worker.
-- Returns 0 rows if job doesn't exist or ownership was lost.
-- Note: available_at is set to completed_at since NOT NULL constraint prevents NULL.
UPDATE recurring_generation_jobs
SET status = 'completed',
    completed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    available_at = NOW()
WHERE id = $1 AND claimed_by = $2;

-- name: ScheduleJobRetry :execrows
-- Reschedule job for retry with incremented retry count.
-- Only succeeds if job is still owned by the specified worker.
UPDATE recurring_generation_jobs
SET status = 'pending',
    retry_count = $2,
    error_message = $3,
    scheduled_for = $4,
    failed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    available_at = $4  -- Match scheduled_for: job available when scheduled (not before due to backoff)
WHERE id = $1 AND claimed_by = $5;

-- name: DiscardJobAfterMaxRetries :execrows
-- Move job to discarded state after exhausting retries.
UPDATE recurring_generation_jobs
SET status = 'discarded',
    retry_count = $2,
    error_message = $3,
    failed_at = NOW()
WHERE id = $1;

-- name: DiscardJobWithOwnershipCheck :execrows
-- Mark job as discarded with ownership verification.
-- Used by MoveToDeadLetter for atomic DLQ + discard operation.
-- Returns 0 rows if job doesn't exist or ownership was lost.
UPDATE recurring_generation_jobs
SET status = 'discarded',
    error_message = $3,
    failed_at = NOW(),
    claimed_by = NULL,
    claimed_at = NULL,
    available_at = NOW()
WHERE id = $1 AND claimed_by = $2;

-- name: ExtendJobAvailability :execrows
-- Extend the availability timeout for a running job (heartbeat).
-- Only succeeds if job is still owned by the specified worker.
UPDATE recurring_generation_jobs
SET available_at = $3
WHERE id = $1 AND claimed_by = $2 AND status = 'running';

-- name: CancelPendingJob :execrows
-- Cancel a pending or scheduled job immediately.
-- Returns 0 rows if job doesn't exist or is not cancellable.
UPDATE recurring_generation_jobs
SET status = 'cancelled'
WHERE id = $1 AND status IN ('pending', 'scheduled');

-- name: RequestCancellationForRunningJob :execrows
-- Request cancellation for a running job (sets cancelling status).
-- Worker must cooperatively stop processing when it sees this status.
UPDATE recurring_generation_jobs
SET status = 'cancelling'
WHERE id = $1 AND status = 'running';

-- name: MarkJobAsCancelled :execrows
-- Final cancellation by worker after cooperative shutdown.
-- Note: available_at is set to NOW() since NOT NULL constraint prevents NULL.
UPDATE recurring_generation_jobs
SET status = 'cancelled',
    claimed_by = NULL,
    claimed_at = NULL,
    available_at = NOW()
WHERE id = $1 AND status = 'cancelling' AND claimed_by = $2;

-- name: HasPendingOrRunningJob :one
-- Check if a template has any pending, running, or scheduled job.
-- Used to prevent duplicate job creation.
SELECT EXISTS (
    SELECT 1 FROM recurring_generation_jobs
    WHERE template_id = $1
      AND status IN ('pending', 'scheduled', 'running')
);

-- name: InsertDeadLetterJob :exec
-- Move a failed job to the dead letter queue for admin review.
INSERT INTO dead_letter_jobs (
    original_job_id, template_id, generate_from, generate_until,
    error_type, error_message, stack_trace,
    retry_count, last_worker_id, original_scheduled_for, original_created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10, $11
);

-- name: ListPendingDeadLetterJobs :many
-- Retrieve unresolved dead letter jobs for admin review.
-- Ordered by failure time (most recent first).
SELECT * FROM dead_letter_jobs
WHERE resolution IS NULL
ORDER BY failed_at DESC
LIMIT $1;

-- name: GetDeadLetterJob :one
-- Retrieve a specific dead letter job by ID.
SELECT * FROM dead_letter_jobs
WHERE id = $1;

-- name: MarkDeadLetterAsRetried :execrows
-- Mark a dead letter job as retried by admin.
UPDATE dead_letter_jobs
SET resolution = 'retried',
    reviewed_at = NOW(),
    reviewed_by = $2
WHERE id = $1 AND resolution IS NULL;

-- name: MarkDeadLetterAsDiscarded :execrows
-- Mark a dead letter job as discarded with admin note.
UPDATE dead_letter_jobs
SET resolution = 'discarded',
    reviewed_at = NOW(),
    reviewed_by = $2,
    reviewer_note = $3
WHERE id = $1 AND resolution IS NULL;

-- name: DeleteResolvedDeadLetterJobs :execrows
-- Cleanup old resolved dead letter jobs (housekeeping).
-- Retention period determined by caller (e.g., 30 days).
DELETE FROM dead_letter_jobs
WHERE resolution IS NOT NULL
  AND reviewed_at < $1;

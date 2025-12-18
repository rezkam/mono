-- name: CreateGenerationJob :exec
-- Creates a new generation job. For immediate scheduling, pass NULL for scheduled_for
-- to use the database's transaction timestamp (DEFAULT now()). This prevents clock skew
-- between the application and database from making jobs temporarily unclaimable.
-- For future scheduling, pass an explicit timestamp to override the default.
INSERT INTO recurring_generation_jobs (
    id, template_id, scheduled_for, status,
    generate_from, generate_until, created_at
) VALUES (
    $1, $2, COALESCE($3, now()), $4, $5, $6, $7
);

-- name: GetGenerationJob :one
SELECT * FROM recurring_generation_jobs
WHERE id = $1;

-- name: ListPendingGenerationJobs :many
SELECT * FROM recurring_generation_jobs
WHERE status = 'PENDING' AND scheduled_for <= $1
ORDER BY scheduled_for ASC
LIMIT $2;

-- name: UpdateGenerationJobStatus :exec
UPDATE recurring_generation_jobs
SET status = $1,
    started_at = CASE WHEN $1 = 'RUNNING' THEN $2 ELSE started_at END,
    completed_at = CASE WHEN $1 = 'COMPLETED' THEN $2 ELSE completed_at END,
    failed_at = CASE WHEN $1 = 'FAILED' THEN $2 ELSE failed_at END,
    error_message = $3,
    retry_count = $4
WHERE id = $5;

-- name: DeleteCompletedGenerationJobs :exec
DELETE FROM recurring_generation_jobs
WHERE status = 'COMPLETED' AND completed_at < $1;

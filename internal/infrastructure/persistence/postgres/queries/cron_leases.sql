-- name: TryAcquireLease :one
-- Atomically try to acquire or renew a lease for exclusive execution.
-- Uses INSERT ON CONFLICT to handle both initial acquisition and renewal.
-- Returns the lease if successfully acquired/renewed, NULL otherwise.
INSERT INTO cron_job_leases (
    run_type, holder_id, acquired_at, expires_at, renewed_at, run_count
) VALUES (
    $1, $2, NOW(), $3, NOW(), 1
)
ON CONFLICT (run_type) DO UPDATE
SET holder_id = EXCLUDED.holder_id,
    expires_at = EXCLUDED.expires_at,
    renewed_at = NOW(),
    run_count = cron_job_leases.run_count + 1
WHERE cron_job_leases.expires_at <= NOW()
   OR cron_job_leases.holder_id = EXCLUDED.holder_id
RETURNING *;

-- name: ReleaseLease :execrows
-- Release a lease held by the specified holder.
-- Only succeeds if the lease is currently held by this holder.
DELETE FROM cron_job_leases
WHERE run_type = $1 AND holder_id = $2;

-- name: RenewLease :execrows
-- Renew an existing lease by extending its expiration.
-- Only succeeds if the lease is currently held by this holder.
UPDATE cron_job_leases
SET expires_at = $3,
    renewed_at = NOW()
WHERE run_type = $1 AND holder_id = $2;

-- name: GetLease :one
-- Retrieve the current lease holder for a run type.
SELECT * FROM cron_job_leases
WHERE run_type = $1;

-- name: CleanupExpiredLeases :execrows
-- Remove expired leases (housekeeping).
DELETE FROM cron_job_leases
WHERE expires_at <= NOW();

-- +goose Up
-- +goose StatementBegin

-- Add unique partial index to prevent duplicate active jobs per template.
--
-- Problem: The scheduler code assumes a unique constraint exists to prevent
-- race conditions when multiple workers schedule jobs for the same template.
-- Without this constraint, concurrent HasPendingOrRunningJob() checks can
-- both return false, leading to duplicate job insertions.
--
-- Solution: Partial unique index on template_id for active job statuses.
-- This allows:
--   - Multiple completed/failed/discarded jobs per template (for history)
--   - Only ONE active (pending/scheduled/running) job per template
--
-- Note: Cannot use CREATE INDEX CONCURRENTLY inside a transaction block,
-- so we use regular CREATE UNIQUE INDEX. For production with existing data,
-- consider running this migration during low-traffic periods.

CREATE UNIQUE INDEX idx_generation_jobs_unique_active_per_template
ON recurring_generation_jobs(template_id)
WHERE status IN ('pending', 'scheduled', 'running');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_generation_jobs_unique_active_per_template;

-- +goose StatementEnd

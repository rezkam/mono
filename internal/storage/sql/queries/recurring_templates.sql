-- name: CreateRecurringTemplate :exec
INSERT INTO recurring_task_templates (
    id, list_id, title, tags, priority, estimated_duration,
    recurrence_pattern, recurrence_config, due_offset,
    is_active, created_at, updated_at,
    last_generated_until, generation_window_days
) VALUES (
    sqlc.arg(id), sqlc.arg(list_id), sqlc.arg(title), sqlc.arg(tags), sqlc.arg(priority),
    sqlc.narg('estimated_duration'),
    sqlc.arg(recurrence_pattern), sqlc.arg(recurrence_config),
    sqlc.narg('due_offset'),
    sqlc.arg(is_active), sqlc.arg(created_at), sqlc.arg(updated_at),
    sqlc.arg(last_generated_until), sqlc.arg(generation_window_days)
);

-- name: GetRecurringTemplate :one
SELECT * FROM recurring_task_templates
WHERE id = $1;

-- name: ListRecurringTemplates :many
SELECT * FROM recurring_task_templates
WHERE list_id = $1 AND is_active = true
ORDER BY created_at DESC;

-- name: ListAllRecurringTemplatesByList :many
SELECT * FROM recurring_task_templates
WHERE list_id = $1
ORDER BY created_at DESC;

-- name: ListAllActiveRecurringTemplates :many
SELECT * FROM recurring_task_templates
WHERE is_active = true
ORDER BY created_at DESC;

-- name: UpdateRecurringTemplate :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) where int64 is the number of rows affected
--
-- Why this pattern:
--   - Single database round-trip (vs two-query SELECT+UPDATE pattern)
--   - No race condition: record cannot be deleted between check and update
--   - Efficient: PostgreSQL returns affected count with no additional cost
--   - Repository layer checks: rowsAffected == 0 → domain.ErrNotFound
--
-- Anti-pattern to avoid:
--   SELECT to check existence, then UPDATE if found
--   - Two round-trips to database
--   - Race condition window between queries
--   - Doubled network latency
UPDATE recurring_task_templates
SET title = sqlc.arg(title),
    tags = sqlc.arg(tags),
    priority = sqlc.arg(priority),
    estimated_duration = sqlc.narg('estimated_duration'),
    recurrence_pattern = sqlc.arg(recurrence_pattern),
    recurrence_config = sqlc.arg(recurrence_config),
    due_offset = sqlc.narg('due_offset'),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: UpdateRecurringTemplateGenerationWindow :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Critical for worker: Detects if template was deleted between job claim and generation
UPDATE recurring_task_templates
SET last_generated_until = $1,
    updated_at = $2
WHERE id = $3;

-- name: DeactivateRecurringTemplate :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Soft delete with existence detection in single operation
UPDATE recurring_task_templates
SET is_active = false,
    updated_at = $1
WHERE id = $2;

-- name: DeleteRecurringTemplate :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Hard delete with built-in existence verification
DELETE FROM recurring_task_templates
WHERE id = $1;

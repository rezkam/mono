-- name: CreateRecurringTemplate :one
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
)
RETURNING *;

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

-- name: UpdateRecurringTemplate :one
-- FIELD MASK PATTERN: Selective field updates with CASE expressions
-- Only updates fields where set_<field> = true (field mask support)
-- CONCURRENCY: Optional version check for optimistic locking
-- Returns NULL if:
--   - Template doesn't exist
--   - Version mismatch (when expected_version provided)
UPDATE recurring_task_templates
SET title = CASE WHEN sqlc.arg('set_title')::boolean THEN sqlc.narg('title') ELSE title END,
    tags = CASE WHEN sqlc.arg('set_tags')::boolean THEN sqlc.narg('tags') ELSE tags END,
    priority = CASE WHEN sqlc.arg('set_priority')::boolean THEN sqlc.narg('priority') ELSE priority END,
    estimated_duration = CASE WHEN sqlc.arg('set_estimated_duration')::boolean THEN sqlc.narg('estimated_duration') ELSE estimated_duration END,
    recurrence_pattern = CASE WHEN sqlc.arg('set_recurrence_pattern')::boolean THEN sqlc.narg('recurrence_pattern') ELSE recurrence_pattern END,
    recurrence_config = CASE WHEN sqlc.arg('set_recurrence_config')::boolean THEN sqlc.narg('recurrence_config') ELSE recurrence_config END,
    due_offset = CASE WHEN sqlc.arg('set_due_offset')::boolean THEN sqlc.narg('due_offset') ELSE due_offset END,
    is_active = CASE WHEN sqlc.arg('set_is_active')::boolean THEN sqlc.narg('is_active') ELSE is_active END,
    generation_window_days = CASE WHEN sqlc.arg('set_generation_window_days')::boolean THEN sqlc.narg('generation_window_days') ELSE generation_window_days END,
    updated_at = NOW(),
    version = version + 1
WHERE id = sqlc.arg('id')
  AND (sqlc.narg('expected_version')::integer IS NULL OR version = sqlc.narg('expected_version')::integer)
RETURNING *;

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

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

-- name: UpdateRecurringTemplate :one
-- DATA ACCESS PATTERN: Partial update with COALESCE pattern
-- Supports field masks by passing NULL for unchanged fields
-- Returns updated row, or pgx.ErrNoRows if template doesn't exist
-- TYPE SAFETY: All fields managed by sqlc - schema changes caught at compile time
--
-- Why this pattern:
--   - Single database round-trip with full row returned
--   - No race condition: atomically updates and returns result
--   - Partial update support: NULL preserves existing values via COALESCE
--   - Type safe: sqlc validates all columns against actual schema
UPDATE recurring_task_templates
SET title = COALESCE(sqlc.narg('title'), title),
    tags = COALESCE(sqlc.narg('tags'), tags),
    priority = COALESCE(sqlc.narg('priority'), priority),
    estimated_duration = COALESCE(sqlc.narg('estimated_duration'), estimated_duration),
    recurrence_pattern = COALESCE(sqlc.narg('recurrence_pattern'), recurrence_pattern),
    recurrence_config = COALESCE(sqlc.narg('recurrence_config'), recurrence_config),
    due_offset = COALESCE(sqlc.narg('due_offset'), due_offset),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    generation_window_days = COALESCE(sqlc.narg('generation_window_days'), generation_window_days),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
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

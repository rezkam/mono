-- name: CreateRecurringTemplate :one
INSERT INTO recurring_task_templates (
    id, list_id, title, tags, priority, estimated_duration,
    recurrence_pattern, recurrence_config, due_offset,
    is_active, created_at, updated_at,
    generated_through, sync_horizon_days, generation_horizon_days
) VALUES (
    sqlc.arg(id), sqlc.arg(list_id), sqlc.arg(title), sqlc.arg(tags), sqlc.arg(priority),
    sqlc.narg('estimated_duration'),
    sqlc.arg(recurrence_pattern), sqlc.arg(recurrence_config),
    sqlc.narg('due_offset'),
    sqlc.arg(is_active), sqlc.arg(created_at), sqlc.arg(updated_at),
    sqlc.arg(generated_through), sqlc.arg(sync_horizon_days), sqlc.arg(generation_horizon_days)
)
RETURNING *;

-- name: FindRecurringTemplateByID :one
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
-- Field mask pattern with optimistic locking support
UPDATE recurring_task_templates
SET title = CASE WHEN sqlc.arg('set_title')::boolean THEN sqlc.narg('title') ELSE title END,
    tags = CASE WHEN sqlc.arg('set_tags')::boolean THEN sqlc.narg('tags') ELSE tags END,
    priority = CASE WHEN sqlc.arg('set_priority')::boolean THEN sqlc.narg('priority') ELSE priority END,
    estimated_duration = CASE WHEN sqlc.arg('set_estimated_duration')::boolean THEN sqlc.narg('estimated_duration') ELSE estimated_duration END,
    recurrence_pattern = CASE WHEN sqlc.arg('set_recurrence_pattern')::boolean THEN sqlc.narg('recurrence_pattern') ELSE recurrence_pattern END,
    recurrence_config = CASE WHEN sqlc.arg('set_recurrence_config')::boolean THEN sqlc.narg('recurrence_config') ELSE recurrence_config END,
    due_offset = CASE WHEN sqlc.arg('set_due_offset')::boolean THEN sqlc.narg('due_offset') ELSE due_offset END,
    is_active = CASE WHEN sqlc.arg('set_is_active')::boolean THEN sqlc.narg('is_active') ELSE is_active END,
    sync_horizon_days = CASE WHEN sqlc.arg('set_sync_horizon_days')::boolean THEN sqlc.narg('sync_horizon_days') ELSE sync_horizon_days END,
    generation_horizon_days = CASE WHEN sqlc.arg('set_generation_horizon_days')::boolean THEN sqlc.narg('generation_horizon_days') ELSE generation_horizon_days END,
    updated_at = NOW(),
    version = version + 1
WHERE id = sqlc.arg('id')
  AND (sqlc.narg('expected_version')::integer IS NULL OR version = sqlc.narg('expected_version')::integer)
RETURNING *;

-- name: SetGeneratedThrough :execrows
UPDATE recurring_task_templates
SET generated_through = $1,
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
DELETE FROM recurring_task_templates
WHERE id = $1;

-- name: FindStaleTemplates :many
-- Find templates that need generation (generated_through < target date)
SELECT * FROM recurring_task_templates
WHERE list_id = $1
  AND is_active = true
  AND generated_through < $2
ORDER BY created_at;

-- name: FindStaleTemplatesForReconciliation :many
-- Find templates needing reconciliation across all lists.
-- Used by reconciliation worker to ensure all templates are properly generated.
-- Excludes:
--   - Templates updated after updated_before (grace period for newly created/updated)
--   - Templates with pending/running jobs (if exclude_pending is true)
--   - Templates already generated through their target date
SELECT t.* FROM recurring_task_templates t
WHERE t.is_active = true
  AND t.generated_through < sqlc.arg('target_date')
  AND t.updated_at <= sqlc.arg('updated_before')
  AND (
    NOT sqlc.arg('exclude_pending')::boolean
    OR NOT EXISTS (
      SELECT 1 FROM recurring_generation_jobs j
      WHERE j.template_id = t.id
        AND j.status IN ('pending', 'running')
    )
  )
ORDER BY t.generated_through ASC, t.created_at ASC
LIMIT CASE WHEN sqlc.arg('limit')::integer = 0 THEN NULL ELSE sqlc.arg('limit')::integer END;

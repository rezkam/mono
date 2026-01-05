-- name: CreateTodoItem :one
INSERT INTO todo_items (
    id, list_id, title, status, priority,
    estimated_duration, actual_duration,
    created_at, updated_at, due_at, tags,
    recurring_template_id, starts_at, occurs_at, due_offset, timezone
) VALUES (
    sqlc.arg(id), sqlc.arg(list_id), sqlc.arg(title), sqlc.arg(status), sqlc.arg(priority),
    sqlc.narg('estimated_duration'), sqlc.narg('actual_duration'),
    sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.narg(due_at), sqlc.arg(tags),
    sqlc.narg(recurring_template_id), sqlc.narg(starts_at), sqlc.narg(occurs_at), sqlc.narg(due_offset), sqlc.narg(timezone)
)
RETURNING *;

-- name: BatchCreateTodoItems :copyfrom
-- Bulk insert using PostgreSQL COPY protocol for high performance
INSERT INTO todo_items (
    id, list_id, title, status, priority,
    estimated_duration, actual_duration,
    created_at, updated_at, due_at, tags,
    recurring_template_id, starts_at, occurs_at, due_offset, timezone,
    version
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17
);

-- name: GetTodoItem :one
SELECT * FROM todo_items
WHERE id = $1;

-- name: GetTodoItemsByListId :many
SELECT * FROM todo_items
WHERE list_id = $1
ORDER BY created_at ASC;

-- name: GetAllTodoItems :many
SELECT * FROM todo_items
ORDER BY list_id, created_at ASC;

-- name: UpdateTodoItem :one
-- DATA ACCESS PATTERN: Partial update with explicit flags
-- Supports field masks by passing boolean flags for fields to update
-- Returns updated row, or pgx.ErrNoRows if:
--   - Item doesn't exist
--   - Item belongs to different list (security: prevents cross-list updates)
--   - Version mismatch (concurrency: prevents lost updates)
-- SECURITY: Validates item belongs to the specified list
-- CONCURRENCY: Optional version check for optimistic locking
-- TYPE SAFETY: All fields managed by sqlc - schema changes caught at compile time
UPDATE todo_items
SET title = CASE WHEN sqlc.arg('set_title')::boolean THEN sqlc.narg('title') ELSE title END,
    status = CASE WHEN sqlc.arg('set_status')::boolean THEN sqlc.narg('status') ELSE status END,
    priority = CASE WHEN sqlc.arg('set_priority')::boolean THEN sqlc.narg('priority') ELSE priority END,
    estimated_duration = CASE WHEN sqlc.arg('set_estimated_duration')::boolean THEN sqlc.narg('estimated_duration') ELSE estimated_duration END,
    actual_duration = CASE WHEN sqlc.arg('set_actual_duration')::boolean THEN sqlc.narg('actual_duration') ELSE actual_duration END,
    due_at = CASE WHEN sqlc.arg('set_due_at')::boolean THEN sqlc.narg('due_at') ELSE due_at END,
    tags = CASE WHEN sqlc.arg('set_tags')::boolean THEN sqlc.narg('tags') ELSE tags END,
    timezone = CASE WHEN sqlc.arg('set_timezone')::boolean THEN sqlc.narg('timezone') ELSE timezone END,
    recurring_template_id = CASE WHEN sqlc.arg('detach_from_template')::boolean THEN NULL ELSE recurring_template_id END,
    updated_at = NOW(),
    version = version + 1
WHERE id = sqlc.arg('id')
  AND list_id = sqlc.arg('list_id')
  AND (sqlc.narg('expected_version')::integer IS NULL OR version = sqlc.narg('expected_version')::integer)
RETURNING *;

-- name: UpdateTodoItemStatus :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Efficient status updates without separate existence check
UPDATE todo_items
SET status = $1, updated_at = $2
WHERE id = $3;

-- name: DeleteTodoItem :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Single-query delete with existence detection built-in
DELETE FROM todo_items
WHERE id = $1;

-- name: DeleteTodoItemsByListId :exec
DELETE FROM todo_items
WHERE list_id = $1;

-- name: CountTasksWithFilters :one
-- Counts total matching items for pagination (used when main query returns empty page).
-- Uses same WHERE clause as ListTasksWithFilters for consistency.
-- $2: statuses array (empty array skips filter, OR logic within array)
-- $3: priorities array (empty array skips filter, OR logic within array)
-- $4: tags array (empty array skips filter, item must have ALL specified tags)
-- $9: excluded_statuses array (empty array skips filter, excludes matching statuses)
SELECT COUNT(*) FROM todo_items
WHERE
    ($1::uuid = '00000000-0000-0000-0000-000000000000' OR list_id = $1) AND
    (array_length($2::text[], 1) IS NULL OR status = ANY($2::text[])) AND
    (array_length($9::text[], 1) IS NULL OR status != ALL($9::text[])) AND
    (array_length($3::text[], 1) IS NULL OR priority = ANY($3::text[])) AND
    (array_length($4::text[], 1) IS NULL OR tags @> $4::text[]) AND
    ($5::timestamptz = '0001-01-01 00:00:00+00' OR due_at <= $5) AND
    ($6::timestamptz = '0001-01-01 00:00:00+00' OR due_at >= $6) AND
    ($7::timestamptz = '0001-01-01 00:00:00+00' OR updated_at >= $7) AND
    ($8::timestamptz = '0001-01-01 00:00:00+00' OR created_at >= $8);

-- name: ListTasksWithFilters :many
-- Optimized for SEARCH/FILTER access pattern: Database-level filtering, sorting, and pagination.
-- Performance: Pushes all operations to PostgreSQL with proper indexes vs loading all items to memory.
-- Use case: Task search, filtered views, "My Tasks" views, pagination through large result sets.
--
-- Parameters (use empty arrays for NULL to skip filters):
--   $1: list_id           - Filter by specific list (zero UUID to search all lists)
--   $2: statuses          - Array of statuses to include (empty array to skip, OR logic)
--   $3: priorities        - Array of priorities to include (empty array to skip, OR logic)
--   $4: tags              - Array of tags to match (empty array to skip, item must have ALL tags)
--   $5: due_before        - Filter tasks due before timestamp (zero time to skip)
--   $6: due_after         - Filter tasks due after timestamp (zero time to skip)
--   $7: updated_at        - Filter by last update time (zero time to skip)
--   $8: created_at        - Filter by creation time (zero time to skip)
--   $9: order_by          - Combined field+direction: 'due_at_asc', 'due_at_desc', etc.
--                           Supports: due_at, priority, created_at, updated_at with _asc or _desc suffix
--                           For bare field names, defaults are: due_at=asc, priority=asc,
--                           created_at=desc, updated_at=desc
--   $10: limit            - Page size (max items to return)
--   $11: offset           - Pagination offset (skip N items)
--   $12: excluded_statuses - Array of statuses to exclude (empty array to skip filter)
--                           Used to exclude archived/cancelled by default when $2 is empty
--
-- Returns: All todo_items columns plus total_count (total matching rows across all pages)
-- The COUNT(*) OVER() window function computes total matching rows in a single query pass,
-- enabling accurate pagination UI without a separate count query.
--
-- SQL Injection Protection:
-- The ORDER BY clause uses parameterized queries ($9::text) with CASE expressions.
-- PostgreSQL's parameterized query protocol treats $9 as DATA, never CODE, making SQL
-- injection structurally impossible. Even if malicious input like "id; DROP TABLE--"
-- flows through without validation, it's compared as a string literal in CASE expressions,
-- never executed as SQL. This protection is guaranteed by PostgreSQL's wire protocol.
-- See tests/integration/sql_injection_resistance_test.go for proof.
--
-- Input validation at the service layer improves UX (clear error messages) but does NOT
-- provide security - parameterized queries are the security boundary.
--
-- Access pattern examples:
--   - "Show my overdue tasks": filter by due_before=now, order by due_at_asc
--   - "Active work": statuses=[todo, in_progress], default sort
--   - "High priority items": priorities=[high, urgent], order by due_at_asc
--   - "Tasks tagged 'urgent' and 'work'": tags=[urgent, work] (item must have both)
SELECT i.*, COUNT(*) OVER() AS total_count
FROM todo_items i
LEFT JOIN recurring_template_exceptions e
    ON i.recurring_template_id = e.template_id
    AND i.occurs_at = e.occurs_at
WHERE
    e.id IS NULL AND  -- Exclude items with exceptions
    ($1::uuid = '00000000-0000-0000-0000-000000000000' OR i.list_id = $1) AND
    (array_length($2::text[], 1) IS NULL OR i.status = ANY($2::text[])) AND
    (array_length($12::text[], 1) IS NULL OR i.status != ALL($12::text[])) AND
    (array_length($3::text[], 1) IS NULL OR i.priority = ANY($3::text[])) AND
    (array_length($4::text[], 1) IS NULL OR i.tags @> $4::text[]) AND
    ($5::timestamptz = '0001-01-01 00:00:00+00' OR i.due_at <= $5) AND
    ($6::timestamptz = '0001-01-01 00:00:00+00' OR i.due_at >= $6) AND
    ($7::timestamptz = '0001-01-01 00:00:00+00' OR i.updated_at >= $7) AND
    ($8::timestamptz = '0001-01-01 00:00:00+00' OR i.created_at >= $8)
ORDER BY
    -- due_at: default ASC
    CASE WHEN $9::text IN ('due_at', 'due_at_asc') THEN i.due_at END ASC NULLS LAST,
    CASE WHEN $9::text = 'due_at_desc' THEN i.due_at END DESC NULLS LAST,
    -- priority: default ASC (semantic order: low=1 < medium=2 < high=3 < urgent=4)
    -- Uses numeric weights instead of lexical ordering to match proto enum semantics
    CASE WHEN $9::text IN ('priority', 'priority_asc') THEN
        CASE i.priority
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'urgent' THEN 4
        END
    END ASC NULLS LAST,
    CASE WHEN $9::text = 'priority_desc' THEN
        CASE i.priority
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'urgent' THEN 4
        END
    END DESC NULLS LAST,
    -- created_at: default DESC
    CASE WHEN $9::text = 'created_at_asc' THEN i.created_at END ASC,
    CASE WHEN $9::text IN ('created_at', 'created_at_desc') THEN i.created_at END DESC,
    -- updated_at: default DESC
    CASE WHEN $9::text = 'updated_at_asc' THEN i.updated_at END ASC,
    CASE WHEN $9::text IN ('updated_at', 'updated_at_desc') THEN i.updated_at END DESC,
    -- Fallback: created_at DESC (when no valid order_by specified)
    i.created_at DESC
LIMIT $10
OFFSET $11;

-- name: InsertItemIgnoreConflict :exec
-- Idempotent single insert with ON CONFLICT DO NOTHING
-- Used in batch operations - duplicates silently ignored based on UNIQUE(recurring_template_id, occurs_at)
INSERT INTO todo_items (
    id, list_id, title, status, priority,
    estimated_duration, actual_duration,
    created_at, updated_at, due_at, tags,
    recurring_template_id, starts_at, occurs_at, due_offset, timezone,
    version
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17
)
ON CONFLICT (recurring_template_id, occurs_at) WHERE recurring_template_id IS NOT NULL
DO NOTHING;

-- name: DeleteFuturePendingItems :execrows
-- Delete future pending items for a template (used before regeneration)
DELETE FROM todo_items
WHERE recurring_template_id = $1
  AND occurs_at >= $2
  AND status = 'todo';

-- name: CreateTodoItem :exec
INSERT INTO todo_items (
    id, list_id, title, status, priority,
    estimated_duration, actual_duration,
    create_time, updated_at, due_time, tags,
    recurring_template_id, instance_date, timezone,
    version
) VALUES (
    sqlc.arg(id), sqlc.arg(list_id), sqlc.arg(title), sqlc.arg(status), sqlc.arg(priority),
    sqlc.narg('estimated_duration'), sqlc.narg('actual_duration'),
    sqlc.arg(create_time), sqlc.arg(updated_at), sqlc.narg(due_time), sqlc.arg(tags),
    sqlc.narg(recurring_template_id), sqlc.narg(instance_date), sqlc.narg(timezone),
    1
);

-- name: GetTodoItem :one
SELECT * FROM todo_items
WHERE id = $1;

-- name: GetTodoItemsByListId :many
SELECT * FROM todo_items
WHERE list_id = $1
ORDER BY create_time ASC;

-- name: GetAllTodoItems :many
SELECT * FROM todo_items
ORDER BY list_id, create_time ASC;

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
    due_time = CASE WHEN sqlc.arg('set_due_time')::boolean THEN sqlc.narg('due_time') ELSE due_time END,
    tags = CASE WHEN sqlc.arg('set_tags')::boolean THEN sqlc.narg('tags') ELSE tags END,
    timezone = CASE WHEN sqlc.arg('set_timezone')::boolean THEN sqlc.narg('timezone') ELSE timezone END,
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
    (array_length($4::text[], 1) IS NULL OR tags ?& $4::text[]) AND
    ($5::timestamptz = '0001-01-01 00:00:00+00' OR due_time <= $5) AND
    ($6::timestamptz = '0001-01-01 00:00:00+00' OR due_time >= $6) AND
    ($7::timestamptz = '0001-01-01 00:00:00+00' OR updated_at >= $7) AND
    ($8::timestamptz = '0001-01-01 00:00:00+00' OR create_time >= $8);

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
--   $9: order_by          - Combined field+direction: 'due_time_asc', 'due_time_desc', etc.
--                           Supports: due_time, priority, created_at, updated_at with _asc or _desc suffix
--                           For bare field names, defaults are: due_time=asc, priority=asc,
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
--   - "Show my overdue tasks": filter by due_before=now, order by due_time_asc
--   - "Active work": statuses=[todo, in_progress], default sort
--   - "High priority items": priorities=[high, urgent], order by due_time_asc
--   - "Tasks tagged 'urgent' and 'work'": tags=[urgent, work] (item must have both)
SELECT *, COUNT(*) OVER() AS total_count FROM todo_items
WHERE
    ($1::uuid = '00000000-0000-0000-0000-000000000000' OR list_id = $1) AND
    (array_length($2::text[], 1) IS NULL OR status = ANY($2::text[])) AND
    (array_length($12::text[], 1) IS NULL OR status != ALL($12::text[])) AND
    (array_length($3::text[], 1) IS NULL OR priority = ANY($3::text[])) AND
    (array_length($4::text[], 1) IS NULL OR tags ?& $4::text[]) AND
    ($5::timestamptz = '0001-01-01 00:00:00+00' OR due_time <= $5) AND
    ($6::timestamptz = '0001-01-01 00:00:00+00' OR due_time >= $6) AND
    ($7::timestamptz = '0001-01-01 00:00:00+00' OR updated_at >= $7) AND
    ($8::timestamptz = '0001-01-01 00:00:00+00' OR create_time >= $8)
ORDER BY
    -- due_time: default ASC
    CASE WHEN $9::text IN ('due_time', 'due_time_asc') THEN due_time END ASC NULLS LAST,
    CASE WHEN $9::text = 'due_time_desc' THEN due_time END DESC NULLS LAST,
    -- priority: default ASC (semantic order: low=1 < medium=2 < high=3 < urgent=4)
    -- Uses numeric weights instead of lexical ordering to match proto enum semantics
    CASE WHEN $9::text IN ('priority', 'priority_asc') THEN
        CASE priority
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'urgent' THEN 4
        END
    END ASC NULLS LAST,
    CASE WHEN $9::text = 'priority_desc' THEN
        CASE priority
            WHEN 'low' THEN 1
            WHEN 'medium' THEN 2
            WHEN 'high' THEN 3
            WHEN 'urgent' THEN 4
        END
    END DESC NULLS LAST,
    -- created_at: default DESC
    CASE WHEN $9::text = 'created_at_asc' THEN create_time END ASC,
    CASE WHEN $9::text IN ('created_at', 'created_at_desc') THEN create_time END DESC,
    -- updated_at: default DESC
    CASE WHEN $9::text = 'updated_at_asc' THEN updated_at END ASC,
    CASE WHEN $9::text IN ('updated_at', 'updated_at_desc') THEN updated_at END DESC,
    -- Fallback: created_at DESC (when no valid order_by specified)
    create_time DESC
LIMIT $10
OFFSET $11;

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

-- name: UpdateTodoItem :execrows
-- DATA ACCESS PATTERN: Optimistic locking with version check
-- :execrows returns (int64, error) - Repository checks rowsAffected:
--   0 → Either item doesn't exist, belongs to different list, OR version mismatch (concurrent update)
--   1 → Success, version incremented
-- SECURITY: Validates item belongs to the specified list to prevent cross-list updates
-- CONCURRENCY: Version check prevents lost updates in race conditions
UPDATE todo_items
SET title = sqlc.arg(title),
    status = sqlc.arg(status),
    priority = sqlc.arg(priority),
    estimated_duration = sqlc.narg('estimated_duration'),
    actual_duration = sqlc.narg('actual_duration'),
    updated_at = sqlc.arg(updated_at),
    due_time = sqlc.narg(due_time),
    tags = sqlc.arg(tags),
    timezone = sqlc.narg(timezone),
    version = version + 1
WHERE id = sqlc.arg(id)
  AND list_id = sqlc.arg(list_id)
  AND version = sqlc.arg(version);

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
SELECT COUNT(*) FROM todo_items
WHERE
    ($1::uuid = '00000000-0000-0000-0000-000000000000' OR list_id = $1) AND
    ($2::text = '' OR status = $2) AND
    ($3::text = '' OR priority = $3) AND
    ($4::text = '' OR tags ? $4::text) AND
    ($5::timestamptz = '0001-01-01 00:00:00+00' OR due_time <= $5) AND
    ($6::timestamptz = '0001-01-01 00:00:00+00' OR due_time >= $6) AND
    ($7::timestamptz = '0001-01-01 00:00:00+00' OR updated_at >= $7) AND
    ($8::timestamptz = '0001-01-01 00:00:00+00' OR create_time >= $8);

-- name: ListTasksWithFilters :many
-- Optimized for SEARCH/FILTER access pattern: Database-level filtering, sorting, and pagination.
-- Performance: Pushes all operations to PostgreSQL with proper indexes vs loading all items to memory.
-- Use case: Task search, filtered views, "My Tasks" views, pagination through large result sets.
--
-- Parameters (use zero values for NULL to skip filters):
--   $1: list_id     - Filter by specific list (zero UUID to search all lists)
--   $2: status      - Filter by status (empty string to skip)
--   $3: priority    - Filter by priority (empty string to skip)
--   $4: tag         - Filter by tag (JSONB array contains, empty string to skip)
--   $5: due_before  - Filter tasks due before timestamp (zero time to skip)
--   $6: due_after   - Filter tasks due after timestamp (zero time to skip)
--   $7: updated_at  - Filter by last update time (zero time to skip)
--   $8: created_at  - Filter by creation time (zero time to skip)
--   $9: order_by    - Combined field+direction: 'due_time_asc', 'due_time_desc', etc.
--                     Supports: due_time, priority, created_at, updated_at with _asc or _desc suffix
--                     For bare field names, defaults are: due_time=asc, priority=asc,
--                     created_at=desc, updated_at=desc
--   $10: limit      - Page size (max items to return)
--   $11: offset     - Pagination offset (skip N items)
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
-- Access pattern example:
--   - "Show my overdue tasks": filter by due_before=now, order by due_time_asc
--   - "Tasks in List X": filter by list_id, default sort
--   - "High priority items": filter by priority=HIGH, order by due_time_asc
--   - "Tasks tagged 'urgent'": filter by tag=urgent (uses GIN index)
SELECT *, COUNT(*) OVER() AS total_count FROM todo_items
WHERE
    ($1::uuid = '00000000-0000-0000-0000-000000000000' OR list_id = $1) AND
    ($2::text = '' OR status = $2) AND
    ($3::text = '' OR priority = $3) AND
    ($4::text = '' OR tags ? $4::text) AND
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

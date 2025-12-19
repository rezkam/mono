-- name: CreateTodoItem :exec
INSERT INTO todo_items (
    id, list_id, title, status, priority,
    estimated_duration, actual_duration,
    create_time, updated_at, due_time, tags,
    recurring_template_id, instance_date, timezone
) VALUES (
    sqlc.arg(id), sqlc.arg(list_id), sqlc.arg(title), sqlc.arg(status), sqlc.arg(priority),
    sqlc.narg('estimated_duration'), sqlc.narg('actual_duration'),
    sqlc.arg(create_time), sqlc.arg(updated_at), sqlc.narg(due_time), sqlc.arg(tags),
    sqlc.narg(recurring_template_id), sqlc.narg(instance_date), sqlc.narg(timezone)
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
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Single database round-trip prevents race conditions and reduces latency
UPDATE todo_items
SET title = sqlc.arg(title),
    status = sqlc.arg(status),
    priority = sqlc.arg(priority),
    estimated_duration = sqlc.narg('estimated_duration'),
    actual_duration = sqlc.narg('actual_duration'),
    updated_at = sqlc.arg(updated_at),
    due_time = sqlc.narg(due_time),
    tags = sqlc.arg(tags),
    timezone = sqlc.narg(timezone)
WHERE id = sqlc.arg(id);

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
--   $9: order_by    - Sort field: 'due_time', 'priority', 'created_at', 'updated_at'
--   $10: limit      - Page size (max items to return)
--   $11: offset     - Pagination offset (skip N items)
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
--   - "Show my overdue tasks": filter by due_before=now, order by due_time
--   - "Tasks in List X": filter by list_id, default sort
--   - "High priority items": filter by priority=HIGH, order by due_time
--   - "Tasks tagged 'urgent'": filter by tag=urgent (uses GIN index)
SELECT * FROM todo_items
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
    CASE WHEN $9::text = 'due_time' THEN due_time END ASC NULLS LAST,
    CASE WHEN $9::text = 'priority' THEN priority END ASC NULLS LAST,
    CASE WHEN $9::text = 'created_at' THEN create_time END DESC,
    CASE WHEN $9::text = 'updated_at' THEN updated_at END DESC,
    create_time DESC
LIMIT $10
OFFSET $11;

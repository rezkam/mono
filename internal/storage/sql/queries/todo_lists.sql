-- name: CreateTodoList :exec
INSERT INTO todo_lists (id, title, create_time)
VALUES ($1, $2, $3);

-- name: GetTodoList :one
SELECT * FROM todo_lists
WHERE id = $1;

-- name: UpdateTodoList :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Avoids two-query anti-pattern (SELECT then UPDATE) with race condition and doubled latency
UPDATE todo_lists
SET title = $1, create_time = $2
WHERE id = $3;

-- name: DeleteTodoList :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Efficient detection of non-existent records without separate SELECT query
DELETE FROM todo_lists
WHERE id = $1;

-- name: ListTodoLists :many
-- Legacy query: Returns all lists without items (use ListTodoListsWithCounts for list views).
SELECT * FROM todo_lists
ORDER BY create_time DESC;

-- name: ListTodoListsWithCounts :many
-- Optimized for LIST VIEW access pattern: Returns list metadata with item counts.
-- Performance: O(lists + items) with single aggregation query vs O(lists * items) loading all items.
-- Use case: Dashboard/overview pages showing list summaries without loading full item details.
-- 
-- Returns:
--   - total_items: Total count of all items in the list
--   - undone_items: Count of active items (TODO, IN_PROGRESS, BLOCKED)
-- 
-- This query uses LEFT JOIN to ensure lists with zero items still appear with count=0.
-- The FILTER clause efficiently counts only active items in a single pass.
SELECT 
    tl.id,
    tl.title,
    tl.create_time,
    COALESCE(COUNT(ti.id), 0)::int AS total_items,
    COALESCE(COUNT(ti.id) FILTER (WHERE ti.status IN ('TODO', 'IN_PROGRESS', 'BLOCKED')), 0)::int AS undone_items
FROM todo_lists tl
LEFT JOIN todo_items ti ON tl.id = ti.list_id
GROUP BY tl.id, tl.title, tl.create_time
ORDER BY tl.create_time DESC;

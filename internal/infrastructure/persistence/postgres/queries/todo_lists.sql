-- name: CreateTodoList :one
INSERT INTO todo_lists (id, title, create_time)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTodoList :one
SELECT * FROM todo_lists
WHERE id = @id;

-- name: GetTodoListWithCounts :one
-- Returns a single list by ID with item counts (for detail view).
-- undone_statuses parameter: domain layer defines which statuses count as "undone".
SELECT
    tl.id,
    tl.title,
    tl.create_time,
    tl.version,
    COALESCE(COUNT(ti.id), 0)::int AS total_items,
    COALESCE(COUNT(ti.id) FILTER (WHERE ti.status = ANY(@undone_statuses::text[])), 0)::int AS undone_items
FROM todo_lists tl
LEFT JOIN todo_items ti ON tl.id = ti.list_id
WHERE tl.id = @id
GROUP BY tl.id, tl.title, tl.create_time, tl.version;

-- name: UpdateTodoList :one
-- ATOMIC UPDATE WITH COUNTS: Uses CTE to update and return counts in single statement.
-- Prevents race conditions where counts could change between UPDATE and SELECT.
--
-- FIELD MASK PATTERN: Selective field updates with CASE expressions
-- Only updates fields where set_<field> = true (field mask support)
-- CONCURRENCY: Optional version check for optimistic locking
-- Returns no rows if:
--   - List doesn't exist
--   - Version mismatch (when expected_version provided)
WITH updated AS (
    UPDATE todo_lists tl
    SET title = CASE WHEN sqlc.arg('set_title')::boolean THEN sqlc.narg('title') ELSE title END,
        version = tl.version + 1
    WHERE tl.id = sqlc.arg('id')
      AND (sqlc.narg('expected_version')::integer IS NULL OR tl.version = sqlc.narg('expected_version')::integer)
    RETURNING *
)
SELECT
    u.id,
    u.title,
    u.create_time,
    u.version,
    COALESCE(COUNT(ti.id), 0)::int AS total_items,
    COALESCE(COUNT(ti.id) FILTER (WHERE ti.status = ANY(@undone_statuses::text[])), 0)::int AS undone_items
FROM updated u
LEFT JOIN todo_items ti ON u.id = ti.list_id
GROUP BY u.id, u.title, u.create_time, u.version;

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
--   - undone_items: Count of items matching provided statuses (domain defines "undone")
--
-- This query uses LEFT JOIN to ensure lists with zero items still appear with count=0.
-- The FILTER clause efficiently counts only matching items in a single pass.
SELECT
    tl.id,
    tl.title,
    tl.create_time,
    tl.version,
    COALESCE(COUNT(ti.id), 0)::int AS total_items,
    COALESCE(COUNT(ti.id) FILTER (WHERE ti.status = ANY(@undone_statuses::text[])), 0)::int AS undone_items
FROM todo_lists tl
LEFT JOIN todo_items ti ON tl.id = ti.list_id
GROUP BY tl.id, tl.title, tl.create_time, tl.version
ORDER BY tl.create_time DESC;

-- name: FindTodoListsWithFilters :many
-- Advanced list query with filtering, sorting, and pagination.
-- Supports AIP-160-style filtering and AIP-132-style sorting.
--
-- Parameters use nullable types for optional filters:
--   - title_contains: Filters by title substring (case-insensitive)
--   - create_time_after: Filters lists created after this time
--   - create_time_before: Filters lists created before this time
--   - order_by: Column to sort by ("create_time" or "title")
--   - order_dir: Sort direction ("asc" or "desc")
--   - page_limit: Maximum number of results to return
--   - page_offset: Number of results to skip
SELECT
    tl.id,
    tl.title,
    tl.create_time,
    tl.version,
    COALESCE(COUNT(ti.id), 0)::int AS total_items,
    COALESCE(COUNT(ti.id) FILTER (WHERE ti.status = ANY(@undone_statuses::text[])), 0)::int AS undone_items
FROM todo_lists tl
LEFT JOIN todo_items ti ON tl.id = ti.list_id
WHERE
    (@title_contains::text IS NULL OR LOWER(tl.title) LIKE LOWER('%' || @title_contains || '%'))
    AND (@create_time_after::timestamptz IS NULL OR tl.create_time > @create_time_after)
    AND (@create_time_before::timestamptz IS NULL OR tl.create_time < @create_time_before)
GROUP BY tl.id, tl.title, tl.create_time, tl.version
ORDER BY
    CASE
        WHEN @order_by = 'title' AND @order_dir = 'asc' THEN tl.title
    END ASC,
    CASE
        WHEN @order_by = 'title' AND @order_dir = 'desc' THEN tl.title
    END DESC,
    CASE
        WHEN @order_by = 'create_time' AND @order_dir = 'asc' THEN tl.create_time
        WHEN (@order_by IS NULL OR @order_by = '') AND (@order_dir IS NULL OR @order_dir = '' OR @order_dir = 'asc') THEN tl.create_time
    END ASC,
    CASE
        WHEN @order_by = 'create_time' AND @order_dir = 'desc' THEN tl.create_time
        WHEN (@order_by IS NULL OR @order_by = '') AND @order_dir = 'desc' THEN tl.create_time
    END DESC
LIMIT @page_limit
OFFSET @page_offset;

-- name: CountTodoListsWithFilters :one
-- Count total matching lists for pagination (same filters as FindTodoListsWithFilters).
SELECT COUNT(DISTINCT tl.id)::int AS total_count
FROM todo_lists tl
WHERE
    (@title_contains::text IS NULL OR LOWER(tl.title) LIKE LOWER('%' || @title_contains || '%'))
    AND (@create_time_after::timestamptz IS NULL OR tl.create_time > @create_time_after)
    AND (@create_time_before::timestamptz IS NULL OR tl.create_time < @create_time_before);

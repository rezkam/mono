-- name: CreateTodoItem :exec
INSERT INTO todo_items (id, list_id, title, completed, create_time, due_time, tags)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetTodoItemsByListId :many
SELECT id, list_id, title, completed, create_time, due_time, tags
FROM todo_items
WHERE list_id = $1
ORDER BY create_time ASC;

-- name: UpdateTodoItem :exec
UPDATE todo_items
SET title = $1, completed = $2, due_time = $3, tags = $4
WHERE id = $5 AND list_id = $6;

-- name: DeleteTodoItem :exec
DELETE FROM todo_items
WHERE id = $1 AND list_id = $2;

-- name: DeleteTodoItemsByListId :exec
DELETE FROM todo_items
WHERE list_id = $1;

-- name: GetAllTodoItems :many
SELECT id, list_id, title, completed, create_time, due_time, tags
FROM todo_items
ORDER BY list_id, create_time ASC;

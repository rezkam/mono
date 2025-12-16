-- name: CreateTodoList :exec
INSERT INTO todo_lists (id, title, create_time)
VALUES ($1, $2, $3);

-- name: GetTodoList :one
SELECT id, title, create_time
FROM todo_lists
WHERE id = $1;

-- name: UpdateTodoList :exec
UPDATE todo_lists
SET title = $1, create_time = $2
WHERE id = $3;

-- name: ListTodoLists :many
SELECT id, title, create_time
FROM todo_lists
ORDER BY create_time DESC;

-- name: DeleteTodoList :exec
DELETE FROM todo_lists
WHERE id = $1;

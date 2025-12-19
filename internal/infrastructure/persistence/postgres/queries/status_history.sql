-- name: CreateStatusHistoryEntry :exec
INSERT INTO task_status_history (id, task_id, from_status, to_status, changed_at, notes)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetTaskStatusHistory :many
SELECT * FROM task_status_history
WHERE task_id = $1
ORDER BY changed_at DESC;

-- name: GetTaskStatusHistoryByDateRange :many
SELECT * FROM task_status_history
WHERE task_id = $1 AND changed_at BETWEEN $2 AND $3
ORDER BY changed_at ASC;

-- name: CreateException :one
INSERT INTO recurring_template_exceptions (
    id,
    template_id,
    occurs_at,
    exception_type,
    item_id,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: FindExceptions :many
SELECT * FROM recurring_template_exceptions
WHERE template_id = $1
  AND occurs_at BETWEEN $2 AND $3
ORDER BY occurs_at;

-- name: FindExceptionByOccurrence :one
SELECT * FROM recurring_template_exceptions
WHERE template_id = $1 AND occurs_at = $2;

-- name: DeleteException :exec
DELETE FROM recurring_template_exceptions
WHERE template_id = $1 AND occurs_at = $2;

-- name: ListAllExceptionsByTemplate :many
SELECT * FROM recurring_template_exceptions
WHERE template_id = $1
ORDER BY occurs_at;

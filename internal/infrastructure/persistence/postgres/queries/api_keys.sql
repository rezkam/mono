-- name: CreateAPIKey :exec
INSERT INTO api_keys (id, key_type, service, version, short_token, long_secret_hash, name, is_active, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetAPIKeyByShortToken :one
SELECT * FROM api_keys
WHERE short_token = $1 AND is_active = true;

-- name: UpdateAPIKeyLastUsed :execrows
-- Updates last_used_at only if the new timestamp is later than the current value.
-- Returns 0 rows affected if: (1) key doesn't exist, OR (2) timestamp not later.
-- Repository uses CheckAPIKeyExists to distinguish these cases.
UPDATE api_keys
SET last_used_at = $2
WHERE id = $1
  AND (last_used_at IS NULL OR last_used_at < $2);

-- name: CheckAPIKeyExists :one
-- Checks if an API key exists by ID.
-- Used by UpdateLastUsed to distinguish "not found" from "timestamp not later".
SELECT EXISTS(SELECT 1 FROM api_keys WHERE id = $1);

-- name: DeactivateAPIKey :execrows
-- DATA ACCESS PATTERN: Single-query existence check via rowsAffected
-- :execrows returns (int64, error) - Repository checks rowsAffected == 0 → domain.ErrNotFound
-- Revokes API key with existence check in single operation
UPDATE api_keys
SET is_active = false
WHERE id = $1;

-- name: ListActiveAPIKeys :many
SELECT * FROM api_keys
WHERE is_active = true
ORDER BY created_at DESC;

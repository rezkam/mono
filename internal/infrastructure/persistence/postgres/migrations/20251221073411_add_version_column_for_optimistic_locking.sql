-- +goose Up
-- +goose StatementBegin
-- Add version column for optimistic locking to prevent lost updates
-- Default to 1 for existing rows, future inserts will use 1 as well
ALTER TABLE todo_items ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

-- Add comment explaining the purpose
COMMENT ON COLUMN todo_items.version IS 'Optimistic locking version - incremented on each update to detect concurrent modifications';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE todo_items DROP COLUMN version;
-- +goose StatementEnd

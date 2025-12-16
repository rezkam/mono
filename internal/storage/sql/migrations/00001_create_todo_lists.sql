-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS todo_lists (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS todo_lists;
-- +goose StatementEnd

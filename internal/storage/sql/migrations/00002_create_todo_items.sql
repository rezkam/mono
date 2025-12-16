-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS todo_items (
    id TEXT PRIMARY KEY,
    list_id TEXT NOT NULL,
    title TEXT NOT NULL,
    completed INTEGER NOT NULL DEFAULT 0,
    create_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    due_time TIMESTAMP,
    tags TEXT,
    FOREIGN KEY (list_id) REFERENCES todo_lists(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_todo_items_list_id ON todo_items(list_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_todo_items_list_id;
DROP TABLE IF EXISTS todo_items;
-- +goose StatementEnd

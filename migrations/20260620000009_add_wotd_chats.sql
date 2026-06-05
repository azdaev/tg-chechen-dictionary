-- +goose Up
CREATE TABLE wotd_chats (
    chat_id INTEGER PRIMARY KEY,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE wotd_chats;

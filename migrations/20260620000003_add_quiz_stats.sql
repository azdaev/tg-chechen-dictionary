-- +goose Up
-- Per-user quiz score, powering the running "Счёт: correct/total" feedback in
-- /quiz. Gamifying progress encourages repeated vocabulary practice.
CREATE TABLE IF NOT EXISTS quiz_stats (
    user_id INTEGER PRIMARY KEY,
    correct_count INTEGER NOT NULL DEFAULT 0,
    total_count INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS quiz_stats;

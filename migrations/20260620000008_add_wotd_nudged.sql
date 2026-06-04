-- +goose Up
ALTER TABLE users ADD COLUMN wotd_nudged INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN wotd_nudged;

-- +goose Up
-- Players without a Telegram @username all rendered as identical "Аноним"
-- rows on /top. Telegram always supplies a first name, so store it as the
-- fallback display name.
ALTER TABLE quiz_stats ADD COLUMN first_name TEXT;

-- +goose Down
ALTER TABLE quiz_stats DROP COLUMN first_name;

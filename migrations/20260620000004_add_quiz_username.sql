-- +goose Up
-- Store the player's username alongside their quiz score so the /top
-- leaderboard can display names without a join to the users table (quiz-only
-- players may never have triggered a translation that records them there).
ALTER TABLE quiz_stats ADD COLUMN username TEXT;

-- +goose Down
ALTER TABLE quiz_stats DROP COLUMN username;

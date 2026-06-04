-- +goose Up
-- Daily practice streak: consecutive days with at least one quiz answer.
-- Streaks are the strongest habit mechanic for daily vocabulary practice.
ALTER TABLE quiz_stats ADD COLUMN streak_days INTEGER NOT NULL DEFAULT 0;
ALTER TABLE quiz_stats ADD COLUMN last_answer_date TEXT;

-- +goose Down
ALTER TABLE quiz_stats DROP COLUMN streak_days;
ALTER TABLE quiz_stats DROP COLUMN last_answer_date;

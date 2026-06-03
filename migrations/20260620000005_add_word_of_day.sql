-- +goose Up
-- Opt-in flag for the daily "Word of the Day" push. Default 0 so nobody is
-- subscribed until they explicitly opt in via /wotd.
ALTER TABLE users ADD COLUMN word_of_day_subscribed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN word_of_day_subscribed;

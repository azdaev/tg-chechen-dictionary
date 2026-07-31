-- +goose Up
-- +goose StatementBegin
-- dosham weights every entry (observed tiers: 16, 100, 10000) and ranking uses
-- that weight to order results. Without a stored copy the signal lives only as
-- long as the Redis entry: once the cache expires the answer is rebuilt from
-- this table, every pair ties at zero, and ordering silently degrades to
-- length and alphabet. Existing rows start at 0 and are backfilled by
-- InsertTranslationPair the next time the word is looked up.
alter table dictionary_pairs add column rate integer not null default 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table dictionary_pairs drop column rate;
-- +goose StatementEnd

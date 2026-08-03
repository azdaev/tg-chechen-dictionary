-- +goose Up
-- +goose StatementBegin
-- Holds an ArticleStructure as JSON for the one corpus that stores a whole
-- entry as a single string. Filled by cmd/parse_articles, offline and once:
-- the corpus is finite and static, so the model never runs at render time —
-- which is what made the retired AI formatter produce a competing layout.
-- Empty means the card falls back to the regex parser; the card's shape is the
-- same either way, only the quality of what fills it changes.
alter table dictionary_pairs add column structured_json text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table dictionary_pairs drop column structured_json;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
-- Spelling-insensitive keys: the clean word minus the combining marks and the
-- palochka, neither of which is on any keyboard. «чӏегӏардиг» and «чегардиг»
-- fold to the same string, so a query that dropped the palochka still lands on
-- a word we already know. Nullable on purpose: null means "not backfilled yet".
alter table dictionary_pairs add column original_folded text;
alter table dictionary_pairs add column translation_folded text;
create index if not exists idx_dictionary_pairs_original_folded
    on dictionary_pairs (original_folded);
create index if not exists idx_dictionary_pairs_translation_folded
    on dictionary_pairs (translation_folded);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists idx_dictionary_pairs_translation_folded;
drop index if exists idx_dictionary_pairs_original_folded;
alter table dictionary_pairs drop column translation_folded;
alter table dictionary_pairs drop column original_folded;
-- +goose StatementEnd

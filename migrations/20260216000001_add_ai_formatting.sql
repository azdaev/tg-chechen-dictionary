-- +goose Up
-- +goose StatementBegin
-- Add AI formatting columns to dictionary_pairs
alter table dictionary_pairs add column formatted_ai text;
alter table dictionary_pairs add column formatted_chosen text;
alter table dictionary_pairs add column format_version text default 'legacy';

-- Index for filtering by format version
create index if not exists idx_dictionary_pairs_format_version
    on dictionary_pairs (format_version);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists idx_dictionary_pairs_format_version;
alter table dictionary_pairs drop column formatted_ai;
alter table dictionary_pairs drop column formatted_chosen;
alter table dictionary_pairs drop column format_version;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
create table if not exists dictionary_pairs (
    id integer primary key autoincrement,
    original_raw text not null,
    original_clean text not null,
    original_lang text not null,
    translation_raw text not null,
    translation_clean text not null,
    translation_lang text not null,
    source text not null,
    source_entry_id text,
    source_translation_id text,
    is_approved integer not null default 0,
    moderation_sent_at datetime,
    approved_at datetime,
    approved_by text,
    created_at datetime not null default current_timestamp
);

create unique index if not exists idx_dictionary_pairs_unique
    on dictionary_pairs (
        original_clean,
        original_lang,
        translation_clean,
        translation_lang
    );

create index if not exists idx_dictionary_pairs_original_clean
    on dictionary_pairs (original_clean);

create index if not exists idx_dictionary_pairs_translation_clean
    on dictionary_pairs (translation_clean);

create index if not exists idx_dictionary_pairs_approved
    on dictionary_pairs (is_approved);

create index if not exists idx_dictionary_pairs_moderation_sent
    on dictionary_pairs (moderation_sent_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists idx_dictionary_pairs_approved;
drop index if exists idx_dictionary_pairs_translation_clean;
drop index if exists idx_dictionary_pairs_original_clean;
drop index if exists idx_dictionary_pairs_unique;
drop table if exists dictionary_pairs;
-- +goose StatementEnd

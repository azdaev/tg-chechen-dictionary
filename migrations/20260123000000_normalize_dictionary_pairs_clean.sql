-- +goose Up
-- +goose StatementBegin
create table if not exists dictionary_pairs_new (
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

insert or ignore into dictionary_pairs_new (
    id,
    original_raw,
    original_clean,
    original_lang,
    translation_raw,
    translation_clean,
    translation_lang,
    source,
    source_entry_id,
    source_translation_id,
    is_approved,
    moderation_sent_at,
    approved_at,
    approved_by,
    created_at
)
select
    id,
    original_raw,
    replace(original_clean, 'ё', 'е') as original_clean,
    original_lang,
    translation_raw,
    replace(translation_clean, 'ё', 'е') as translation_clean,
    translation_lang,
    source,
    source_entry_id,
    source_translation_id,
    is_approved,
    moderation_sent_at,
    approved_at,
    approved_by,
    created_at
from dictionary_pairs
order by id;

drop table dictionary_pairs;
alter table dictionary_pairs_new rename to dictionary_pairs;

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
-- no-op
-- +goose StatementEnd

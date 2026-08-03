-- The grammar card prints a word's paradigm («Формы: ло̃ьмаш, ло̃ьман, …») but
-- searching one of those forms found nothing. dosham returns them only in the
-- grammar query, so they live here, keyed by the folded form the user can
-- actually type. The primary key indexes form_folded on its own already.

-- +goose Up
-- +goose StatementBegin
create table if not exists word_forms (
    form_folded text not null,
    headword    text not null,
    primary key (form_folded, headword)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists word_forms;
-- +goose StatementEnd

-- NormalizeSearch now drops the stress mark, and the clean columns hold its
-- output. A row written before keeps «писа́ние», which no typed query equals,
-- so the exact layer answered from the unstressed rows alone and never reached
-- the Russian–Chechen article. Where the re-keyed spelling is already taken,
-- REPLACE drops the unstressed twin: the stressed row is the academic or the
-- article corpus, the better source of the two.

-- +goose Up
-- +goose StatementBegin
update or replace dictionary_pairs
set original_clean = replace(original_clean, char(769), ''),
    translation_clean = replace(translation_clean, char(769), '')
where instr(original_clean, char(769)) > 0 or instr(translation_clean, char(769)) > 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
select 1;
-- +goose StatementEnd

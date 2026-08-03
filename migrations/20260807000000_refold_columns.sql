-- FoldSearch now drops ъ as well, so every value written by the previous
-- backfill is stale: «къолам» was folded to «къолам» and no longer matches the
-- «колам» a user types. Nulling both columns puts the rows back in
-- BackfillFolded's queue; it refills them on the next start.

-- +goose Up
-- +goose StatementBegin
update dictionary_pairs set original_folded = null, translation_folded = null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
select 1;
-- +goose StatementEnd

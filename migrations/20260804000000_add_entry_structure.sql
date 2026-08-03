-- +goose Up
-- +goose StatementBegin
-- The dosham API already marks what the renderer used to re-derive by parsing
-- strings: whether a row is a headword or a collocation (type), its part of
-- speech (subtype), which homonym it is (entryIndex), and its usage note.
-- fetchTranslationsFromAPI dropped all four, so pairs.go guessed structure from
-- the presence of "1)" and "~" — a sniff that misses every short article and
-- cannot tell цӀа¹ (дом) from цӀа² (домой).
--
-- The local table is the steady-state read path, so the columns have to live
-- here too or a word would render one way on first lookup and another way after
-- it was stored. Rows written before this migration keep NULL/0; the card
-- treats those as "unknown" and simply omits the part-of-speech chip, which
-- changes decoration but never the shape of the card.
alter table dictionary_pairs add column entry_type text;
alter table dictionary_pairs add column subtype integer not null default 0;
alter table dictionary_pairs add column entry_index integer not null default 0;
alter table dictionary_pairs add column entry_notes text;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table dictionary_pairs drop column entry_type;
alter table dictionary_pairs drop column subtype;
alter table dictionary_pairs drop column entry_index;
alter table dictionary_pairs drop column entry_notes;
-- +goose StatementEnd

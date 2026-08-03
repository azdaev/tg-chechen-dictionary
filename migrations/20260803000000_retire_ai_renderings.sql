-- +goose Up
-- +goose StatementBegin
-- An approved AI rendering is sent to the user verbatim (pairs.go hasAIFormatting),
-- so it never passes through the card renderer. The two disagree on every
-- convention the renderer settled: the AI prompt puts the Russian side of an
-- example first and prints the headword unbolded, while the renderer bolds the
-- studied language and leads with Chechen. Worse, approved pairs lead their
-- ranking bucket, so the cards that skip the renderer are the ones users see
-- first — «карандаш» and «слово» answered in two different formats in the same
-- minute on 2026-08-03.
--
-- 'ai' is the only value moderation writes besides 'deleted', and it means two
-- things at once: a human kept this pair, and the AI text is what to show. Only
-- the second is being retired. 'lite' keeps the pair moderated — it stays out of
-- the review queue, still counts in /stats, still feeds /random — and routes its
-- display through the renderer.
--
-- formatted_ai is left untouched: nothing here is lost, and turning it back on
-- is the Down step.
update dictionary_pairs set formatted_chosen = 'lite' where formatted_chosen = 'ai';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Exact inverse as of this migration: nothing writes 'lite' yet, so every row
-- carrying it got there from the Up step.
update dictionary_pairs set formatted_chosen = 'ai' where formatted_chosen = 'lite';
-- +goose StatementEnd

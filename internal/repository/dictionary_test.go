package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newDictionaryTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1) // keep the single in-memory connection alive
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE dictionary_pairs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		original_raw TEXT NOT NULL,
		original_clean TEXT NOT NULL,
		original_lang TEXT NOT NULL,
		translation_raw TEXT NOT NULL,
		translation_clean TEXT NOT NULL,
		translation_lang TEXT NOT NULL,
		source TEXT NOT NULL,
		source_entry_id TEXT,
		source_translation_id TEXT,
		formatted_ai TEXT,
		formatted_chosen TEXT
	);
	CREATE UNIQUE INDEX idx_dictionary_pairs_unique
		ON dictionary_pairs (original_clean, original_lang, translation_clean, translation_lang);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewRepository(db)
}

func TestFindTranslationPairs_IncludesUnmoderated(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	pair := TranslationPair{
		OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
		TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS",
		Source: "api",
	}
	id, _, err := r.InsertTranslationPair(ctx, pair)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Fresh pairs have formatted_chosen NULL; NULL != 'deleted' is not true in
	// SQL, so a naive filter hides the entire unmoderated dictionary.
	found, err := r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("FindTranslationPairs(дитт) = %+v (err %v), want the unmoderated pair", found, err)
	}

	// Reverse direction matches on translation_clean and swaps the sides.
	found, err = r.FindTranslationPairs(ctx, "дерево", 10)
	if err != nil || len(found) != 1 || found[0].Original != "Дерево" {
		t.Fatalf("FindTranslationPairs(дерево) = %+v (err %v), want swapped pair", found, err)
	}

	reverse := TranslationPair{
		OriginalRaw: "Дерево", OriginalClean: "дерево", OriginalLang: "RUS",
		TranslationRaw: "Дитт", TranslationClean: "дитт", TranslationLang: "CHE",
		Source: "api",
	}
	reverseID, _, err := r.InsertTranslationPair(ctx, reverse)
	if err != nil {
		t.Fatalf("insert reverse: %v", err)
	}
	found, err = r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 1 || found[0].Original != "Дитт" {
		t.Fatalf("FindTranslationPairs with exact headword = %+v (err %v), want only the headword-side pair", found, err)
	}

	// Deleted pairs stay hidden.
	if err := r.SetTranslationPairFormattingChoice(ctx, id, "deleted"); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if err := r.SetTranslationPairFormattingChoice(ctx, reverseID, "deleted"); err != nil {
		t.Fatalf("mark reverse deleted: %v", err)
	}
	if found, err := r.FindTranslationPairs(ctx, "дитт", 10); err != nil || len(found) != 0 {
		t.Fatalf("FindTranslationPairs after delete = %+v (err %v), want none", found, err)
	}
}

func TestFindTranslationPairsByPrefix(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	insert := func(origRaw, origClean, trRaw, trClean string) {
		t.Helper()
		_, _, err := r.InsertTranslationPair(ctx, TranslationPair{
			OriginalRaw: origRaw, OriginalClean: origClean, OriginalLang: "RUS",
			TranslationRaw: trRaw, TranslationClean: trClean, TranslationLang: "CHE",
			Source: "api",
		})
		if err != nil {
			t.Fatalf("insert %s: %v", origClean, err)
		}
	}
	insert("Яблоко", "яблоко", "Ӏаж", "ӏаж")
	insert("Яблоня", "яблоня", "Ӏаж дитт", "ӏаж дитт")
	insert("Груша", "груша", "Кхор", "кхор")

	found, err := r.FindTranslationPairsByPrefix(ctx, "яблок", 10)
	if err != nil || len(found) != 1 || found[0].Original != "Яблоко" {
		t.Fatalf("prefix яблок = %+v (err %v), want [Яблоко]", found, err)
	}

	// Chechen side matches too, with the sides swapped so the matched word leads.
	found, err = r.FindTranslationPairsByPrefix(ctx, "кхор", 10)
	if err != nil || len(found) != 1 || found[0].Original != "Кхор" {
		t.Fatalf("prefix кхор = %+v (err %v), want [Кхор] (swapped)", found, err)
	}

	if found, err = r.FindTranslationPairsByPrefix(ctx, "слон", 10); err != nil || len(found) != 0 {
		t.Fatalf("prefix слон = %+v (err %v), want none", found, err)
	}
}

func TestInsertTranslationPair_ReportsInserted(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	pair := TranslationPair{
		OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
		TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS",
		Source: "api",
	}

	id1, inserted, err := r.InsertTranslationPair(ctx, pair)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !inserted || id1 == 0 {
		t.Fatalf("first insert: id=%d inserted=%v, want new row", id1, inserted)
	}

	// Insert another pair so the connection's last rowid moves on — a duplicate
	// must still resolve to its own ID, not the connection's latest.
	other := pair
	other.OriginalClean, other.TranslationClean = "хен", "ствол"
	id2, inserted, err := r.InsertTranslationPair(ctx, other)
	if err != nil || !inserted || id2 == id1 {
		t.Fatalf("second insert: id=%d inserted=%v err=%v", id2, inserted, err)
	}

	dupID, inserted, err := r.InsertTranslationPair(ctx, pair)
	if err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}
	if inserted {
		t.Fatal("duplicate insert reported as new")
	}
	if dupID != id1 {
		t.Fatalf("duplicate resolved to id %d, want original %d", dupID, id1)
	}
}

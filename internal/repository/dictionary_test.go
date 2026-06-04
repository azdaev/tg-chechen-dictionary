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
		source_translation_id TEXT
	);
	CREATE UNIQUE INDEX idx_dictionary_pairs_unique
		ON dictionary_pairs (original_clean, original_lang, translation_clean, translation_lang);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewRepository(db)
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

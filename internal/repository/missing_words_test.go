package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newMissingWordsTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE missing_words (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		clean_word TEXT NOT NULL UNIQUE,
		raw_word TEXT NOT NULL,
		search_count INTEGER NOT NULL DEFAULT 1,
		first_searched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_searched_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewRepository(db)
}

func TestMissingWords_RecordAndResolve(t *testing.T) {
	r := newMissingWordsTestRepo(t)
	ctx := context.Background()

	if err := r.RecordMissingWord(ctx, "тест", "Тест"); err != nil {
		t.Fatalf("RecordMissingWord: %v", err)
	}
	if err := r.RecordMissingWord(ctx, "тест", "тест"); err != nil {
		t.Fatalf("RecordMissingWord (repeat): %v", err)
	}

	words, err := r.TopMissingWords(ctx, 10)
	if err != nil || len(words) != 1 || words[0].SearchCount != 2 {
		t.Fatalf("TopMissingWords = %+v (err %v), want one word searched twice", words, err)
	}

	// Once a search succeeds the word is no longer a gap.
	if err := r.ResolveMissingWord(ctx, "тест"); err != nil {
		t.Fatalf("ResolveMissingWord: %v", err)
	}
	if count, err := r.CountMissingWords(ctx); err != nil || count != 0 {
		t.Fatalf("CountMissingWords after resolve = %d (err %v), want 0", count, err)
	}

	// Resolving an absent word is a quiet no-op.
	if err := r.ResolveMissingWord(ctx, "небыло"); err != nil {
		t.Fatalf("ResolveMissingWord (absent): %v", err)
	}
}

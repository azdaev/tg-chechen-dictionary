package repository

import (
	"context"
	"testing"
)

func insertFoldedPair(t *testing.T, r *Repository, origRaw, origClean, transRaw, transClean string) {
	t.Helper()
	if _, _, err := r.InsertTranslationPair(context.Background(), TranslationPair{
		OriginalRaw: origRaw, OriginalClean: origClean, OriginalLang: "CHE",
		TranslationRaw: transRaw, TranslationClean: transClean, TranslationLang: "RUS",
		Source: "api",
	}); err != nil {
		t.Fatalf("insert %q: %v", origClean, err)
	}
}

func TestFindTranslationPairsByFolded(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()
	insertFoldedPair(t, r, "Чӏегӏардиг", "чӏегӏардиг", "Ласточка", "ласточка")

	// The user drops both palochkas; the stored word still answers.
	got, err := r.FindTranslationPairsByFolded(ctx, "чегардиг", 10)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 1 || got[0].Original != "Чӏегӏардиг" {
		t.Fatalf("got %+v, want the stored Chechen headword", got)
	}

	// A reverse hit leads with the side that matched, languages following it.
	rev, err := r.FindTranslationPairsByFolded(ctx, "ласточка", 10)
	if err != nil {
		t.Fatalf("reverse lookup: %v", err)
	}
	if len(rev) != 1 || rev[0].Original != "Ласточка" || rev[0].OriginalLang != "RUS" {
		t.Fatalf("reverse hit = %+v, want Ласточка leading as RUS", rev)
	}

	if got, err := r.FindTranslationPairsByFolded(ctx, "", 10); err != nil || got != nil {
		t.Fatalf("empty key must not scan the table: %+v %v", got, err)
	}
}

func TestBackfillFolded(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	// Rows as they looked before the folded columns existed.
	for _, w := range []string{"цӏа", "гӏала", "кӏант", "бӏаьрг", "тӏай"} {
		if _, err := r.db.ExecContext(ctx,
			`insert into dictionary_pairs
			 (original_raw, original_clean, original_lang, translation_raw, translation_clean, translation_lang, source)
			 values (?, ?, 'CHE', ?, ?, 'RUS', 'api')`,
			w, w, w+"-ru", w+"-ru"); err != nil {
			t.Fatalf("seed %q: %v", w, err)
		}
	}

	n, err := r.BackfillFolded(ctx, 2)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 5 {
		t.Fatalf("filled %d rows, want 5 — batching dropped rows", n)
	}

	var unfilled int
	if err := r.db.QueryRowContext(ctx,
		`select count(*) from dictionary_pairs where original_folded is null`).Scan(&unfilled); err != nil {
		t.Fatalf("count: %v", err)
	}
	if unfilled != 0 {
		t.Fatalf("%d rows left null", unfilled)
	}

	// The fold really is the palochka-less key, not a copy of the clean word.
	got, err := r.FindTranslationPairsByFolded(ctx, "ца", 10)
	if err != nil || len(got) != 1 || got[0].Original != "цӏа" {
		t.Fatalf("backfilled row not reachable by its folded key: %+v %v", got, err)
	}

	// Idempotent: a second start must not rewrite the table.
	again, err := r.BackfillFolded(ctx, 2)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if again != 0 {
		t.Fatalf("second run touched %d rows, want 0", again)
	}
}

func TestInsertTranslationPair_FillsFolded(t *testing.T) {
	r := newDictionaryTestRepo(t)
	// New rows arrive already folded, which is what makes the backfill's
	// `is null` scan safe against concurrent inserts.
	insertFoldedPair(t, r, "Гӏала", "гӏала", "Город", "город")

	var folded string
	if err := r.db.QueryRow(
		`select original_folded from dictionary_pairs where original_clean = 'гӏала'`).Scan(&folded); err != nil {
		t.Fatalf("select: %v", err)
	}
	if folded != "гала" {
		t.Fatalf("original_folded = %q, want гала", folded)
	}
}

package repository

import (
	"chetoru/migrations"
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

	// Run the real migrations instead of hand-building the schema. A hand-built
	// copy drifts from production silently, and main.go log.Fatals on a failed
	// migration — so a broken migration is a restart loop, which no test caught
	// while the schema was duplicated here.
	if err := migrations.Up(db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
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

	// Deleted pairs stay hidden.
	if err := r.SetTranslationPairFormattingChoice(ctx, id, "deleted"); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if found, err := r.FindTranslationPairs(ctx, "дитт", 10); err != nil || len(found) != 0 {
		t.Fatalf("FindTranslationPairs after delete = %+v (err %v), want none", found, err)
	}
}

func TestFindTranslationPairs_ApprovedFirstThenShortest(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	// Inserted worst-first on purpose: without an order by these come back in
	// rowid order, and whatever lands first freezes into the cache.
	var longestID int64
	for i, p := range []TranslationPair{
		{OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
			TranslationRaw: "Дерево, растущее у дома", TranslationClean: "дерево растущее у дома", TranslationLang: "RUS", Source: "api"},
		{OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
			TranslationRaw: "Древо", TranslationClean: "древо", TranslationLang: "RUS", Source: "api"},
		{OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
			TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS", Source: "api"},
	} {
		id, _, err := r.InsertTranslationPair(ctx, p)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if i == 0 {
			longestID = id
		}
	}

	found, err := r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 3 {
		t.Fatalf("FindTranslationPairs = %+v (err %v), want 3 pairs", found, err)
	}
	if found[0].Translate != "Древо" {
		t.Fatalf("first = %q, want the shortest translation", found[0].Translate)
	}

	// A moderator-approved rendering outranks length.
	if err := r.SetTranslationPairFormattingChoice(ctx, longestID, "ai"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	found, err = r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || found[0].Translate != "Дерево, растущее у дома" {
		t.Fatalf("first = %+v (err %v), want the approved pair", found[0], err)
	}
}

// Without a stored rate the ordering signal lives only as long as the Redis
// entry, and the local table is the steady-state read path.
func TestTranslationPairRateSurvivesStorage(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	pair := TranslationPair{
		OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
		TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS",
		Source: "api", Rate: 10000,
	}
	if _, _, err := r.InsertTranslationPair(ctx, pair); err != nil {
		t.Fatalf("insert: %v", err)
	}

	found, err := r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("FindTranslationPairs = %+v (err %v)", found, err)
	}
	if found[0].Rate != 10000 {
		t.Fatalf("rate = %d, want it read back from the row", found[0].Rate)
	}

	// Reverse lookups swap the sides; the rate belongs to the row, not a side.
	if found, err := r.FindTranslationPairs(ctx, "дерево", 10); err != nil || found[0].Rate != 10000 {
		t.Fatalf("reverse lookup rate = %+v (err %v)", found, err)
	}
}

// Rows written before the rate column exists stay at zero forever unless the
// insert path backfills them: a stored pair is never re-inserted.
func TestInsertTranslationPairBackfillsRate(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	pair := TranslationPair{
		OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
		TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS",
		Source: "api",
	}
	if _, _, err := r.InsertTranslationPair(ctx, pair); err != nil {
		t.Fatalf("insert: %v", err)
	}

	pair.Rate = 10000
	id, inserted, err := r.InsertTranslationPair(ctx, pair)
	if err != nil {
		t.Fatalf("re-insert: %v", err)
	}
	if inserted {
		t.Fatal("duplicate reported as newly inserted")
	}

	found, err := r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("FindTranslationPairs = %+v (err %v)", found, err)
	}
	if found[0].Rate != 10000 {
		t.Fatalf("rate = %d for pair %d, want the zero row backfilled", found[0].Rate, id)
	}
}

// The card bolds whichever side is Chechen, so the languages have to swap with
// the sides on a reverse hit. Reading them off the wrong column bolds Russian.
func TestFindTranslationPairsLanguagesFollowOrientation(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	pair := TranslationPair{
		OriginalRaw: "Дитт", OriginalClean: "дитт", OriginalLang: "CHE",
		TranslationRaw: "Дерево", TranslationClean: "дерево", TranslationLang: "RUS",
		Source: "api",
	}
	if _, _, err := r.InsertTranslationPair(ctx, pair); err != nil {
		t.Fatalf("insert: %v", err)
	}

	found, err := r.FindTranslationPairs(ctx, "дитт", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("FindTranslationPairs = %+v (err %v)", found, err)
	}
	if found[0].OriginalLang != "CHE" || found[0].TranslateLang != "RUS" {
		t.Errorf("forward lookup langs = %q/%q, want CHE/RUS", found[0].OriginalLang, found[0].TranslateLang)
	}

	found, err = r.FindTranslationPairs(ctx, "дерево", 10)
	if err != nil || len(found) != 1 {
		t.Fatalf("reverse FindTranslationPairs = %+v (err %v)", found, err)
	}
	if found[0].OriginalLang != "RUS" || found[0].TranslateLang != "CHE" {
		t.Errorf("reverse lookup langs = %q/%q, want RUS/CHE", found[0].OriginalLang, found[0].TranslateLang)
	}

	byPrefix, err := r.FindTranslationPairsByPrefix(ctx, "дерев", 10)
	if err != nil || len(byPrefix) != 1 {
		t.Fatalf("FindTranslationPairsByPrefix = %+v (err %v)", byPrefix, err)
	}
	if byPrefix[0].OriginalLang != "RUS" || byPrefix[0].TranslateLang != "CHE" {
		t.Errorf("prefix lookup langs = %q/%q, want RUS/CHE", byPrefix[0].OriginalLang, byPrefix[0].TranslateLang)
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

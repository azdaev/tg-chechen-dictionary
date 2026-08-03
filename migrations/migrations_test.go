package migrations

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// main.go log.Fatals when migrations fail, so a broken migration is a restart
// loop rather than a degraded start. Nothing exercised the embedded SQL before
// this, which meant the first bad statement would have been found in production.
func TestMigrationsUpDownUp(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1) // keep the single in-memory connection alive
	t.Cleanup(func() { db.Close() })

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// Up is called on every startup and must be a no-op once applied.
	if err := Up(db); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	// The newest migration has to be reversible, or a rollback strands the DB.
	if err := Down(db); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if err := Up(db); err != nil {
		t.Fatalf("Up after Down: %v", err)
	}
}

// The retire migration is the only one that rewrites user-visible data rather
// than schema, so its round trip is worth pinning: an approved pair must stop
// being served verbatim without leaving the moderated set, and Down must put it
// back exactly.
func TestRetireAIRenderings_RoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := Up(db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	insert := func(clean, chosen string) {
		t.Helper()
		_, err := db.Exec(
			`insert into dictionary_pairs
			 (original_raw, original_clean, original_lang, translation_raw, translation_clean,
			  translation_lang, source, formatted_ai, formatted_chosen)
			 values (?, ?, 'RUS', ?, ?, 'CHE', 'api', 'карандаш — къолам', ?)`,
			clean, clean, clean+"-che", clean+"-che", chosen)
		if err != nil {
			t.Fatalf("insert %q: %v", clean, err)
		}
	}
	chosen := func(clean string) string {
		t.Helper()
		var v sql.NullString
		if err := db.QueryRow(`select formatted_chosen from dictionary_pairs where original_clean = ?`, clean).Scan(&v); err != nil {
			t.Fatalf("select %q: %v", clean, err)
		}
		return v.String
	}

	// Down one step so the retire migration can be re-applied over test rows.
	if err := Down(db); err != nil {
		t.Fatalf("Down: %v", err)
	}
	insert("карандаш", "ai")
	insert("удалённая", "deleted")
	if err := Up(db); err != nil {
		t.Fatalf("Up after seeding: %v", err)
	}

	if got := chosen("карандаш"); got != "lite" {
		t.Errorf("formatted_chosen = %q, want lite — the AI text is still being served", got)
	}
	if got := chosen("удалённая"); got != "deleted" {
		t.Errorf("a deleted pair must not be resurrected: %q", got)
	}
	// Retiring the rendering must not retire the moderation: the pair stays out
	// of the review queue and keeps counting as checked.
	var queued int
	if err := db.QueryRow(`select count(*) from dictionary_pairs where formatted_chosen is null`).Scan(&queued); err != nil {
		t.Fatalf("queue count: %v", err)
	}
	if queued != 0 {
		t.Errorf("%d pairs fell back into the moderation queue", queued)
	}
	// The text itself is kept, so Down is a real undo and not a guess.
	var ai string
	if err := db.QueryRow(`select formatted_ai from dictionary_pairs where original_clean = 'карандаш'`).Scan(&ai); err != nil {
		t.Fatalf("select formatted_ai: %v", err)
	}
	if ai == "" {
		t.Error("formatted_ai was wiped; the migration is no longer reversible")
	}

	if err := Down(db); err != nil {
		t.Fatalf("Down after Up: %v", err)
	}
	if got := chosen("карандаш"); got != "ai" {
		t.Errorf("Down left %q, want ai", got)
	}
}

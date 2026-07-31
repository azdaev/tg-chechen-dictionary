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

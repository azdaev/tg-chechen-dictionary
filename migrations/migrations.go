// Package migrations embeds the goose SQL migration files and applies them to
// the application database. The same embedded migrations run automatically on
// app startup (see main.go) and via the cmd/migrate CLI, so production never
// needs a separate manual migration step.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var embedMigrations embed.FS

func configure() error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose.SetDialect: %w", err)
	}
	return nil
}

// Up applies all pending migrations. It is safe to call on every startup: goose
// records applied versions in the goose_db_version table and skips them, so an
// up-to-date database is a no-op.
func Up(db *sql.DB) error {
	if err := configure(); err != nil {
		return err
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose.Up: %w", err)
	}
	return nil
}

// Down rolls back the most recently applied migration.
// DownTo rolls back every migration newer than version. Tests that re-apply a
// specific migration use it instead of counting Down steps, which silently
// broke every time a newer migration was added.
func DownTo(db *sql.DB, version int64) error {
	if err := configure(); err != nil {
		return err
	}
	if err := goose.DownTo(db, ".", version); err != nil {
		return fmt.Errorf("goose.DownTo: %w", err)
	}
	return nil
}

func Down(db *sql.DB) error {
	if err := configure(); err != nil {
		return err
	}
	if err := goose.Down(db, "."); err != nil {
		return fmt.Errorf("goose.Down: %w", err)
	}
	return nil
}

package main

import (
	"database/sql"
	"embed"
	"flag"
	"log"
	"os"

	_ "modernc.org/sqlite"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var embedMigrations embed.FS

func main() {
	isDown := flag.Bool("down", false, "set this flag to run down migrations")
	flag.Parse()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./database.db"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		log.Fatal(err)
	}

	switch {
	case *isDown:
		if err := goose.Down(db, "."); err != nil {
			log.Fatal(err)
		}
	default:
		if err := goose.Up(db, "."); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Migration complete")
}

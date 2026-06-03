// Command migrate applies (or rolls back) database migrations from the CLI.
// The app also runs these same embedded migrations automatically on startup;
// this binary is the manual escape hatch (e.g. `migrate -down`).
package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"chetoru/migrations"

	_ "modernc.org/sqlite"
)

func main() {
	down := flag.Bool("down", false, "roll back the most recent migration instead of applying pending ones")
	flag.Parse()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./database.db"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if *down {
		if err := migrations.Down(db); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := migrations.Up(db); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Migration complete")
}

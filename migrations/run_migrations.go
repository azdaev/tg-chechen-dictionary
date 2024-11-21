// package main

// import (
// 	"database/sql"
// 	"embed"
// 	"fmt"
// 	"log"
// 	"os"

// 	_ "github.com/lib/pq"
// 	"github.com/pressly/goose/v3"
// )

// //go:embed *.sql
// var embedMigrations embed.FS

// func main() {
// 	psqlInfo := fmt.Sprintf(
// 		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
// 		os.Getenv("PG_HOST"),
// 		os.Getenv("PG_PORT"),
// 		os.Getenv("PG_USER"),
// 		os.Getenv("PG_PASSWORD"),
// 		os.Getenv("PG_DB_NAME"),
// 	)

// 	db, err := sql.Open("postgres", psqlInfo)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	goose.SetBaseFS(embedMigrations)

// 	if err := goose.SetDialect("postgres"); err != nil {
// 		log.Fatal(err)
// 	}

// 	if err := goose.Up(db, "."); err != nil {
// 		log.Fatal(err)
// 	}
// }
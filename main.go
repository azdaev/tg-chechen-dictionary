package main

import (
	"chetoru/internal/repository"
	"chetoru/internal/service"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"

	bots "github.com/go-telegram/bot"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log := logrus.New()

	if err := godotenv.Load(); err != nil {
		panic("no .env file found")
	}

	psqlInfo := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("PG_HOST"),
		os.Getenv("PG_PORT"),
		os.Getenv("PG_USER"),
		os.Getenv("PG_PASSWORD"),
		os.Getenv("PG_DB_NAME"),
	)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	usersRepo := repository.NewRepository(db)

	bot, err := bots.New(os.Getenv("TG_BOT_TOKEN"))
	if err != nil {
		panic(err)
	}

	botService := service.NewService(log, usersRepo, bot)
	botService.Start(ctx)
}

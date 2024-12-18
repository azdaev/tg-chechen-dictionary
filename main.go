package main

import (
	"chetoru/internal/business"
	"chetoru/internal/cache"
	"chetoru/internal/net"
	"chetoru/internal/repository"

	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log := logrus.New()

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

	err = db.Ping()
	if err != nil {
		log.Fatal("cannot ping database", err)
	}

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_BOT_TOKEN"))
	if err != nil {
		panic(err)
	}

	bot.Debug = false

	redisCache := cache.NewCache(os.Getenv("REDIS_ADDR"))
	translatorBusiness := business.NewBusiness(redisCache, log)

	botService := net.NewNet(log, usersRepo, bot, translatorBusiness)
	botService.Start(ctx)
}

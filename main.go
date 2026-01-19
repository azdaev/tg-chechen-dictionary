package main

import (
	"chetoru/internal/business"
	"chetoru/internal/cache"
	"chetoru/internal/net"
	"chetoru/internal/repository"

	"context"
	"database/sql"
	"os"
	"os/signal"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log := logrus.New()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./database.db"
	}

	db, err := sql.Open("sqlite3", dbPath)
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
	translatorBusiness := business.NewBusiness(redisCache, usersRepo, log)

	botService := net.NewNet(log, usersRepo, bot, translatorBusiness)
	botService.Start(ctx)
}

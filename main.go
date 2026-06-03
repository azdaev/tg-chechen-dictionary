package main

import (
	"chetoru/internal/ai"
	"chetoru/internal/business"
	"chetoru/internal/cache"
	"chetoru/internal/net"
	"chetoru/internal/repository"
	"chetoru/migrations"

	"context"
	"database/sql"
	"os"
	"os/signal"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "modernc.org/sqlite"
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

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	usersRepo := repository.NewRepository(db)

	err = db.Ping()
	if err != nil {
		log.Fatal("cannot ping database", err)
	}

	// Apply any pending schema migrations before serving. goose tracks applied
	// versions, so an up-to-date database is a no-op; a failure here aborts
	// startup rather than letting the bot run against a stale schema.
	if err := migrations.Up(db); err != nil {
		log.Fatal("failed to apply migrations: ", err)
	}
	log.Info("database migrations up to date")

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_BOT_TOKEN"))
	if err != nil {
		panic(err)
	}

	bot.Debug = false

	redisCache := cache.NewCache(os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"))

	// Initialize AI client (optional)
	var aiClient *ai.Client
	openRouterKey := os.Getenv("OPENROUTER_API_KEY")
	openRouterModel := os.Getenv("OPENROUTER_MODEL")
	if openRouterKey != "" {
		if openRouterModel == "" {
			openRouterModel = "google/gemini-3-flash-preview"
		}
		aiClient = ai.New(openRouterKey, openRouterModel, log)
		log.Printf("AI formatting enabled: %s", openRouterModel)
	} else {
		log.Println("AI formatting disabled (no OPENROUTER_API_KEY)")
	}

	translatorBusiness := business.NewBusiness(redisCache, usersRepo, aiClient, log)

	var spellChecker net.AI
	if aiClient != nil {
		spellChecker = aiClient
	}
	botService := net.NewNet(log, usersRepo, bot, translatorBusiness, redisCache, spellChecker)

	// Wire callback: after AI formatting → send to moderation
	translatorBusiness.SetOnPairReady(func(pairID int64, cleanWord string) {
		botService.SendAutoModeration(context.Background(), cleanWord)
	})

	// Daily "Word of the Day" push to opted-in subscribers.
	botService.StartWordOfDayScheduler(ctx)

	botService.Start(ctx)
}

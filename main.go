package main

import (
	"chetoru/internal/ai"
	"chetoru/internal/business"
	"chetoru/internal/cache"
	"chetoru/internal/net"
	"chetoru/internal/repository"

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

	botService := net.NewNet(log, usersRepo, bot, translatorBusiness)
	botService.Start(ctx)
}

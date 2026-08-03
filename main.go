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
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

func main() {
	// Docker stops containers with SIGTERM; without handling it every redeploy
	// kills the process mid-write instead of letting handlers drain.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log := logrus.New()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./database.db"
	}

	// WAL lets reads proceed during writes; busy_timeout makes a second writer
	// wait instead of failing with SQLITE_BUSY — updates are handled
	// concurrently and background goroutines (AI formatting) write too.
	// synchronous=NORMAL skips the per-commit fsync (WAL syncs at checkpoints
	// instead): corruption-safe, and losing the last few bookkeeping writes on
	// an OS crash is acceptable here.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	repo := repository.NewRepository(db)

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

	// Rows stored before the folded columns existed have to be filled in for the
	// spelling-insensitive lookup to reach them. Detached because the table size
	// on the host is unknown and a startup that waits on it would be a restart
	// risk; a partially filled column is correct for the rows it does hold, so
	// the bot serves normally while this runs and simply covers more as it goes.
	// Closed only on success: a failed backfill leaves rows unfolded, and the
	// bot must keep not caching misses rather than pin a wrong "no translation"
	// for a day. That costs dosham traffic until the next restart, which is the
	// cheaper of the two mistakes.
	backfillOK := make(chan struct{})
	go func() {
		n, err := repo.BackfillFolded(ctx, 500)
		if err != nil {
			log.WithError(err).Warnf("folded backfill stopped after %d rows", n)
			return
		}
		if n > 0 {
			log.Infof("folded backfill filled %d rows", n)
		}
		close(backfillOK)
	}()

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

	translator := business.NewBusiness(redisCache, repo, aiClient, log)
	go func() {
		<-backfillOK
		translator.SetFoldedReady()
	}()

	var spellChecker net.AI
	if aiClient != nil {
		spellChecker = aiClient
	}
	botService := net.NewNet(log, repo, bot, translator, redisCache, spellChecker)

	// Wire callback: after AI formatting → send to moderation
	translator.SetOnPairReady(func(pairID int64, cleanWord string) {
		botService.SendAutoModeration(context.Background(), cleanWord)
	})

	// Warm the word pool so the first /random or /quiz is instant.
	go translator.WarmWordPool(ctx)

	// Daily "Word of the Day" push to opted-in subscribers.
	botService.StartWordOfDayScheduler(ctx)

	// Evening warning to quiz players whose daily streak lapses at midnight.
	botService.StartStreakReminderScheduler(ctx)

	botService.Start(ctx)

	// Bounded grace for detached background work (pair persistence, cache
	// writes) before the deferred db.Close cuts it off.
	done := make(chan struct{})
	go func() {
		botService.WaitBackground()
		translator.WaitBackground()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Warn("background work still running at shutdown")
	}
}

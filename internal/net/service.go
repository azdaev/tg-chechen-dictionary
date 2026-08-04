package net

import (
	"chetoru/internal/cache"
	"sync"

	"context"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type Net struct {
	log      *logrus.Logger
	repo     Repository
	business Business
	ai       AI
	bot      *tgbotapi.BotAPI
	cache    *cache.Cache

	broadcastMu       sync.Mutex
	awaitingBroadcast bool
	pendingBroadcast  *broadcastPayload

	inlineSpellMu     sync.Mutex
	inlineSpellLatest map[int64]string

	quizClaimsMu sync.Mutex
	quizClaims   map[quizKey]struct{}

	// bg tracks detached post-reply work (donation nudge, cache invalidation,
	// missing-word records) so shutdown can wait for it.
	bg sync.WaitGroup
}

// WaitBackground blocks until detached background work has finished.
func (n *Net) WaitBackground() {
	n.bg.Wait()
}

func NewNet(log *logrus.Logger, repo Repository, bot *tgbotapi.BotAPI, business Business, cache *cache.Cache, aiClient AI) *Net {
	return &Net{
		log:               log,
		repo:              repo,
		bot:               bot,
		business:          business,
		ai:                aiClient,
		cache:             cache,
		inlineSpellLatest: make(map[int64]string),
	}
}

// maxConcurrentUpdates bounds how many updates are handled at once. Handlers
// used to run strictly sequentially, so one slow API lookup stalled every
// other user's message.
const maxConcurrentUpdates = 8

// slowUpdateThreshold flags handlers that keep a user waiting long enough to
// notice. AI-backed paths (/check) legitimately take a few seconds; anything
// past this is worth a look in the logs.
const slowUpdateThreshold = 5 * time.Second

// UpdatePollSeconds is how long Telegram holds a getUpdates request open when
// there is nothing to send. Exported because the HTTP client that carries it
// must outlast it — a client timeout below this aborts every idle poll.
const UpdatePollSeconds = 60

func (n *Net) Start(ctx context.Context) {
	n.log.Info("starting service")

	n.registerBotCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = UpdatePollSeconds

	updates := n.bot.GetUpdatesChan(u)

	// dispatch runs a handler concurrently, bounded by the semaphore — when all
	// slots are busy the loop applies backpressure instead of spawning unbounded
	// goroutines. A panicking handler is logged, not fatal. Slow handlers are
	// logged with their kind so optimization targets come from live data.
	sem := make(chan struct{}, maxConcurrentUpdates)
	// Handlers run on a context that survives shutdown: draining is pointless
	// if cancellation makes their remaining DB writes fail.
	handlerCtx := context.WithoutCancel(ctx)
	dispatch := func(kind string, fn func()) {
		sem <- struct{}{}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					n.log.WithField("panic", r).Error("update handler panicked")
				}
				<-sem
			}()
			start := time.Now()
			fn()
			if d := time.Since(start); d > slowUpdateThreshold {
				n.log.WithField("kind", kind).WithField("duration", d.Round(time.Millisecond).String()).Warn("slow update")
			}
		}()
	}

	for {
		var update tgbotapi.Update
		select {
		case <-ctx.Done():
			// Let in-flight handlers finish before main closes the DB: holding
			// every semaphore slot means none are still running.
			n.log.Info("shutting down, draining in-flight handlers")
			for range maxConcurrentUpdates {
				sem <- struct{}{}
			}
			n.log.Info("service stopped")
			return
		case u, ok := <-updates:
			if !ok {
				return
			}
			update = u
		}

		// Callbacks
		if update.CallbackQuery != nil {
			cq := update.CallbackQuery
			dispatch("callback", func() { n.routeCallback(handlerCtx, cq) })
			continue
		}

		// Poll answers (group quiz scoring) — cheap DB write, kept in-loop.
		if update.PollAnswer != nil {
			if err := n.HandlePollAnswer(handlerCtx, update.PollAnswer); err != nil {
				n.log.WithError(err).Error("service.HandlePollAnswer")
			}
			continue
		}

		// Pre-checkout query (Telegram Payments) — must be answered fast and in
		// order, kept in-loop.
		if update.PreCheckoutQuery != nil {
			if err := n.HandlePreCheckout(update.PreCheckoutQuery); err != nil {
				n.log.WithError(err).Error("service.HandlePreCheckout")
			}
			continue
		}

		// Messages
		if update.Message != nil {
			// Successful payment — ordering matters, kept in-loop.
			if update.Message.SuccessfulPayment != nil {
				if err := n.HandleSuccessfulPayment(handlerCtx, update.Message); err != nil {
					n.log.WithError(err).Error("service.HandleSuccessfulPayment")
				}
				continue
			}
			m := update.Message
			dispatch("message", func() { n.routeMessage(handlerCtx, m) })
			continue
		}

		// Inline queries. An empty one is the «@bot » moment in someone else's
		// chat — HandleInline answers it with discovery words, and dropping it
		// here meant that whole branch never ran and the picker stayed blank.
		if update.InlineQuery != nil {
			iq := update.InlineQuery
			dispatch("inline", func() { n.routeInline(handlerCtx, iq) })
			continue
		}
	}
}

// registerBotCommands publishes the user-facing command menu so Telegram shows
// the "/" autocomplete list. Admin-only commands (stats, moderate, broadcast,
// ai) are intentionally omitted. A failure here is non-fatal.
func (n *Net) registerBotCommands() {
	cmds := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "random", Description: "🎲 Случайное чеченское слово"},
		tgbotapi.BotCommand{Command: "quiz", Description: "🧠 Викторина по чеченскому"},
		tgbotapi.BotCommand{Command: "top", Description: "🏆 Рейтинг знатоков"},
		tgbotapi.BotCommand{Command: "me", Description: "👤 Мой прогресс"},
		tgbotapi.BotCommand{Command: "wotd", Description: "📖 Слово дня"},
		tgbotapi.BotCommand{Command: "check", Description: "✍️ Проверить орфографию"},
		tgbotapi.BotCommand{Command: "subscribe", Description: "⭐ Подписка на безлимит"},
	)
	if _, err := n.bot.Request(cmds); err != nil {
		n.log.WithError(err).Warn("failed to register bot commands")
	}
}

func (n *Net) routeCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) {
	data := cq.Data
	var err error

	switch {
	case strings.HasPrefix(data, "broadcast_"):
		err = n.HandleBroadcastCallback(ctx, cq)
	case strings.HasPrefix(data, "more_"):
		err = n.HandleMoreTranslations(ctx, cq)
	case strings.HasPrefix(data, "random_"):
		err = n.HandleRandomCallback(ctx, cq)
	case strings.HasPrefix(data, "quiz_"):
		err = n.HandleQuizCallback(ctx, cq)
	case strings.HasPrefix(data, "wotd_"):
		err = n.HandleWordOfDayCallback(ctx, cq)
	case strings.HasPrefix(data, "check_"):
		err = n.HandleSpellcheckRequest(ctx, cq)
	case strings.HasPrefix(data, "spell_"):
		err = n.HandleSpellcheckFeedback(ctx, cq)
	case strings.HasPrefix(data, "mod_"):
		err = n.HandleModerationCallback(ctx, cq)
	}

	if err != nil {
		n.log.WithError(err).WithField("callback", data).Error("callback handler failed")
	}
}

func (n *Net) routeMessage(ctx context.Context, m *tgbotapi.Message) {
	var err error

	switch m.Command() {
	case "start":
		err = n.HandleStart(m)
	case "stats":
		err = n.HandleStats(ctx, m)
	case "missing":
		err = n.HandleMissingWords(ctx, m)
	case "random":
		err = n.HandleRandom(ctx, m.Chat.ID)
	case "quiz":
		err = n.HandleQuiz(ctx, m.Chat)
	case "top":
		err = n.HandleTop(ctx, m.Chat.ID, m.From.ID)
	case "me":
		err = n.HandleMe(ctx, m)
	case "wotd":
		err = n.HandleWordOfDay(ctx, m)
	case "moderate":
		err = n.HandleModerate(ctx, m)
	case "check":
		err = n.HandleCheck(ctx, m)
	case "subscribe":
		err = n.HandleSubscribe(ctx, m)
	case "ai":
		n.HandleAIToggle(m)
		return
	case "broadcast":
		err = n.HandleBroadcast(ctx, m)
	case "broadcast_cancel":
		err = n.HandleBroadcastCancel(m)
	default:
		// Spellcheck: message starts with "."
		if strings.HasPrefix(m.Text, ".") && len(m.Text) > 1 {
			m.Text = strings.TrimSpace(strings.TrimPrefix(m.Text, "."))
			if m.Text != "" {
				err = n.HandleCheck(ctx, m)
				if err != nil {
					n.log.WithError(err).Error("service.HandleCheck (dot prefix)")
				}
				return
			}
		}

		if n.isAwaitingBroadcastContent(m) {
			err = n.HandleBroadcastContent(m)
		} else {
			err = n.HandleText(ctx, m)
		}
	}

	if err != nil {
		n.log.
			WithField("user_id", m.From.ID).
			WithField("command", m.Command()).
			WithError(err).
			Error("message handler failed")
	}
}

func (n *Net) routeInline(ctx context.Context, iq *tgbotapi.InlineQuery) {
	var err error

	if strings.HasPrefix(iq.Query, ". ") && len(iq.Query) > 2 {
		err = n.HandleInlineSpellcheck(ctx, iq)
	} else {
		err = n.HandleInline(ctx, iq)
	}

	if err != nil {
		n.log.
			WithField("user_id", iq.From.ID).
			WithField("query", iq.Query).
			WithError(err).
			Error("inline handler failed")
	}
}

func (n *Net) isAdmin(userID int64) bool {
	return strconv.Itoa(int(userID)) == os.Getenv("TG_ADMIN_ID")
}

func (n *Net) HandleAIToggle(msg *tgbotapi.Message) {
	if !n.isAdmin(msg.From.ID) {
		return
	}
	switch strings.TrimSpace(msg.CommandArguments()) {
	case "on":
		n.business.SetAIFormatting(true)
		n.send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: ON"))
	case "off":
		n.business.SetAIFormatting(false)
		n.send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: OFF"))
	default:
		status := "OFF"
		if n.business.AIFormattingEnabled() {
			status = "ON"
		}
		n.send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: "+status+"\n/ai on | /ai off"))
	}
}

func (n *Net) isBlockedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "bot was blocked") ||
		strings.Contains(errStr, "user is deactivated") ||
		strings.Contains(errStr, "bot was kicked") ||
		strings.Contains(errStr, "chat not found")
}

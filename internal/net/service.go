package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"sync"

	"context"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

const (
	MaxTranslations         = 4
	InlineResultsLimit      = 50   // Telegram's hard cap on answerInlineQuery results
	InlineDiscoveryCount    = 3    // random words served for an empty inline query
	InlineDiscoveryCacheSec = 300  // short edge cache so the trio rotates
	InlineCacheTimeSeconds  = 3600 // Telegram-side cache for non-personal inline answers
	// No <i>: on a translation card italic marks a usage example and nothing
	// else, and this text sits right under one.
	MoreTranslationsHelpText = `Чтобы просмотреть все доступные переводы, нажмите на кнопку «Ещё» или воспользуйтесь инлайн-режимом: введите @chetoru_bot и слово, которое хотите перевести. Это позволит вам увидеть все варианты.`
	StartMessageText         = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео.\n\n🎲 /random — случайное чеченское слово.\n🧠 /quiz — викторина: проверь, как хорошо ты знаешь чеченский.\n🏆 /top — рейтинг знатоков.\n👤 /me — мой прогресс.\n📖 /wotd — слово дня каждое утро.\n✍️ /check — проверить орфографию (или начни сообщение с точки).\n\nСловарные данные предоставлены проектом dosham.app"
	NoTranslationText        = "К сожалению, нет перевода"
	// Shown when the dictionary itself failed. Saying "нет перевода" there is a
	// lie, and it is the lie that also files the user's word as a vocabulary gap.
	DictionaryUnavailableText = "Словарь сейчас недоступен. Попробуйте через минуту."
	SuggestionsHeaderText     = "🔍 <b>Возможно, вы искали:</b>"
	MoreButtonText            = "Ещё (%d)"
	MissingWordsLimit         = 30
	MissingWordsHeader        = "<b>🔍 Слова без перевода</b>\n\n<i>Слова, которые искали пользователи, но в словаре не нашлось перевода. Это подсказывает, какие слова стоит добавить.</i>\n\n"
	MissingWordsEmpty         = "Пока нет слов без перевода 🎉"
	MissingWordRowFormat      = "%d. <b>%s</b> — %d раз\n"
	RandomWordFormat          = "🎲 <b>Случайное слово</b>\n\n<b>%s</b> — %s"
	RandomMoreButtonText      = "🎲 Ещё одно"
	ShareWordButtonText       = "📤 Поделиться"
	RandomEmptyText           = "Словарь пока пуст. Попробуйте перевести несколько слов, и они появятся здесь!"
	QuizQuestionFormat        = "🧠 <b>Викторина</b>\n\nКак переводится на русский?\n\n<b>%s</b>"
	QuizQuestionReverseFormat = "🧠 <b>Викторина</b>\n\nКак сказать по-чеченски?\n\n<b>%s</b>"
	QuizNextButtonText        = "➡️ Следующий вопрос"
	QuizLookupButtonText      = "📖 Открыть в словаре"
	QuizCorrectToast          = "✅ Верно!"
	QuizWrongToast            = "❌ Неверно"
	QuizErrorText             = "Не удалось составить вопрос. Попробуйте /quiz ещё раз."
	QuizTopLimit              = 10
	QuizTopHeader             = "🏆 <b>Топ знатоков чеченского</b>\n<i>по количеству верных ответов в /quiz</i>\n\n"
	QuizTopEmptyText          = "Пока никто не набрал очков в /quiz. Стань первым! 🧠"
	WordOfDayHour             = 9 // local hour (container TZ is Europe/Moscow)
	WordOfDayFormat           = "📖 <b>Слово дня</b>\n\n<b>%s</b> — %s"
	WordOfDayExampleFormat    = "✍️ <i>%s</i>"
	// No 🇨🇪: CE is unassigned in ISO 3166-1, so it is not a flag anywhere —
	// clients render two letter tiles. And no <i>: the card above already
	// spends italic on its usage example.
	WordOfDayFooter            = "Учите чеченский каждый день!"
	WotdStatusOnText           = "📖 <b>Слово дня</b>\n\nВы подписаны ✅ — каждый день в 9:00 будете получать новое чеченское слово."
	WotdStatusOffText          = "📖 <b>Слово дня</b>\n\nПодпишитесь, чтобы каждое утро получать новое чеченское слово и пополнять словарный запас."
	WotdChatStatusOnText       = "📖 <b>Слово дня</b>\n\nЭтот чат подписан ✅ — каждое утро в 9:00 сюда приходит новое чеченское слово."
	WotdChatStatusOffText      = "📖 <b>Слово дня</b>\n\nПодпишите этот чат, чтобы каждое утро здесь появлялось новое чеченское слово."
	WotdSubscribeButton        = "🔔 Подписаться"
	WotdUnsubscribeButton      = "🔕 Отписаться"
	WotdSubscribedToast        = "Вы подписались на слово дня! 🔔"
	WotdUnsubscribedToast      = "Вы отписались от слова дня"
	WotdNudgeText              = "📖 Кстати! Каждое утро бот может присылать вам одно чеченское слово с переводом — маленький шаг к языку каждый день."
	WotdNudgeMinLookups        = 5
	DonationMessageFormat      = "🌱 Чтобы наш проект мог продолжить работать, вы можете помочь нам"
	DefaultModerationChat      = int64(-5204234916)
	BroadcastParseMode         = "html"
	BroadcastSendDelay         = 100 * time.Millisecond
	StreakReminderHour         = 19 // local hour (container TZ is Europe/Moscow)
	StreakReminderFormat       = "🔥 Ваша серия — <b>%d дн.</b> Один вопрос сегодня, и она продолжится!"
	FreeSpellcheckLimit        = 5
	SubscriptionPriceKopecks   = 10000 // 100 RUB
	SubscriptionPriceFormatted = "100 ₽"
	SubscriptionDuration       = 30 * 24 * time.Hour // 30 days
)

type AI interface {
	SpellCheck(ctx context.Context, text string) (*ai.SpellCheckResult, error)
}

type Business interface {
	Translate(word string) ([]models.TranslationPairs, error)
	SuggestTranslations(word string) []models.TranslationPairs
	SetAIFormatting(enabled bool)
	AIFormattingEnabled() bool
	RandomWordFromAPI(ctx context.Context) (*models.RandomWord, error)
	GenerateQuiz(ctx context.Context) (*models.QuizQuestion, error)
	GrammarFor(ctx context.Context, word string) (*models.WordGrammar, error)
	TranslationCacheStats() (hits, misses int64)
	RecheckTranslation(word string) bool
}

// Repository is the persistence boundary the handlers depend on. It is composed
// from per-domain sub-interfaces so each storage concern can be read (and, in
// tests, mocked) in isolation. The concrete *repository.Repository satisfies the
// whole set; the split is purely for documentation and testability.
type Repository interface {
	UserStore
	StatsStore
	DictionaryStore
	MissingWordStore
	SpellcheckStore
	SubscriptionStore
	QuizStore
	WordOfDayStore
}

// UserStore tracks users, their activity, block state, and donation prompts.
type UserStore interface {
	StoreUser(ctx context.Context, userID int, username string) error
	RecordUserActivity(ctx context.Context, userID int64, username string, activityType models.ActivityType) error
	CountUserActivity(ctx context.Context, userID int64) (int, error)
	ListUserIDs(ctx context.Context) ([]int64, error)
	MarkUserBlocked(ctx context.Context, userID int64, reason string) error
	ShouldSendDonationMessage(ctx context.Context, userID int) (bool, error)
	StoreDonationMessage(ctx context.Context, userID int) error
	WasInlineHinted(ctx context.Context, userID int64) (bool, error)
	MarkInlineHinted(ctx context.Context, userID int64) error
}

// StatsStore answers the aggregate questions behind /stats.
type StatsStore interface {
	CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error)
	DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]models.DailyActivity, error)
	MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error)
}

// DictionaryStore holds the user-contributed translation pairs and their
// moderation/formatting state.
type DictionaryStore interface {
	ListPendingTranslationPairs(ctx context.Context, limit int) ([]repository.TranslationPair, error)
	ListPendingTranslationPairsByWord(ctx context.Context, cleanWord string, limit int) ([]repository.TranslationPair, error)
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	FindStrictlyApprovedPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	GetPairCleanWords(ctx context.Context, pairID int64) ([]string, error)
	RandomApprovedPair(ctx context.Context) (*models.RandomWord, error)
	CountDictionaryPairs(ctx context.Context) (total int, approved int, err error)
}

// MissingWordStore records lookups that found no translation, so maintainers
// know what coverage to add next.
type MissingWordStore interface {
	RecordMissingWord(ctx context.Context, cleanWord, rawWord string) error
	ResolveMissingWord(ctx context.Context, cleanWord string) error
	TopMissingWords(ctx context.Context, limit int) ([]models.MissingWord, error)
	CountMissingWords(ctx context.Context) (int, error)
}

// SpellcheckStore persists spellcheck feedback and per-user monthly usage (for
// the free-tier quota).
type SpellcheckStore interface {
	StoreSpellcheckFeedback(ctx context.Context, userID int64, originalText, correctedText, feedback string) error
	GetSpellcheckUsage(ctx context.Context, userID int64, month, year int) (int, error)
	IncrementSpellcheckUsage(ctx context.Context, userID int64, month, year int) error
}

// SubscriptionStore tracks paid subscriptions purchased via Telegram Payments.
type SubscriptionStore interface {
	HasActiveSubscription(ctx context.Context, userID int64) (bool, error)
	CreateSubscription(ctx context.Context, userID int64, expiresAt time.Time, telegramPaymentID string) error
}

// QuizStore records /quiz answers and the leaderboard behind /top.
type QuizStore interface {
	RecordQuizAnswer(ctx context.Context, userID int64, username, firstName string, correct bool) error
	GetQuizScore(ctx context.Context, userID int64) (correct, total, streak int, err error)
	TopQuizScorers(ctx context.Context, limit int) ([]models.QuizScorer, error)
	GetQuizRank(ctx context.Context, userID int64) (int, error)
	CountQuizStats(ctx context.Context) (players, totalAnswers, correctAnswers int, err error)
	ListLapsingStreaks(ctx context.Context, lastAnswerDate string) ([]models.QuizScorer, error)
	CountActiveStreaks(ctx context.Context) (int, error)
}

// WordOfDayStore manages opt-in subscriptions for the daily "Word of the Day".
type WordOfDayStore interface {
	SetWordOfDaySubscription(ctx context.Context, userID int64, subscribed bool) error
	IsWordOfDaySubscribed(ctx context.Context, userID int64) (bool, error)
	ListWordOfDaySubscribers(ctx context.Context) ([]int64, error)
	CountWordOfDaySubscribers(ctx context.Context) (int, error)
	WasWordOfDayNudged(ctx context.Context, userID int64) (bool, error)
	MarkWordOfDayNudged(ctx context.Context, userID int64) error
	SetChatWordOfDaySubscription(ctx context.Context, chatID int64, subscribed bool) error
	IsChatWordOfDaySubscribed(ctx context.Context, chatID int64) (bool, error)
	ListWordOfDayChatIDs(ctx context.Context) ([]int64, error)
	CountWordOfDayChats(ctx context.Context) (int, error)
}

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

func (n *Net) Start(ctx context.Context) {
	n.log.Info("starting service")

	n.registerBotCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

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

		// Inline queries
		if update.InlineQuery != nil && update.InlineQuery.Query != "" {
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

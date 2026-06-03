package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"

	"context"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

const (
	PathInlineVideo          = "internal/net/inline.mp4"
	MaxTranslations          = 4
	InlineResultsLimit       = 50 // Telegram's hard cap on answerInlineQuery results
	MoreTranslationsHelpText = `<i>Чтобы просмотреть все доступные переводы, нажмите на кнопку «Еще» или воспользуйтесь инлайн-режимом: введите @chetoru_bot и слово, которое хотите перевести. Это позволит вам увидеть все варианты.</i>`
	StartMessageText         = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео.\n\n🎲 /random — случайное чеченское слово.\n🧠 /quiz — викторина: проверь, как хорошо ты знаешь чеченский.\n🏆 /top — рейтинг знатоков.\n📖 /wotd — слово дня каждое утро.\n\nСловарные данные предоставлены проектом dosham.app"
	NoTranslationText        = "К сожалению, нет перевода"
	MoreButtonText           = "Еще (%d)"
	MissingWordsLimit     = 30
	MissingWordsHeader    = "<b>🔍 Слова без перевода</b>\n\n<i>Слова, которые искали пользователи, но в словаре не нашлось перевода. Это подсказывает, какие слова стоит добавить.</i>\n\n"
	MissingWordsEmpty     = "Пока нет слов без перевода 🎉"
	MissingWordRowFormat  = "%d. <b>%s</b> — %d раз\n"
	RandomWordFormat      = "🎲 <b>Случайное слово</b>\n\n<b>%s</b> — %s"
	RandomMoreButtonText  = "🎲 Ещё одно"
	RandomEmptyText       = "Словарь пока пуст. Попробуйте перевести несколько слов, и они появятся здесь!"
	QuizQuestionFormat    = "🧠 <b>Викторина</b>\n\nКак переводится на русский?\n\n<b>%s</b>"
	QuizNextButtonText    = "➡️ Следующий вопрос"
	QuizCorrectToast      = "✅ Верно!"
	QuizWrongToast        = "❌ Неверно"
	QuizErrorText         = "Не удалось составить вопрос. Попробуйте /quiz ещё раз."
	QuizTopLimit          = 10
	QuizTopHeader         = "🏆 <b>Топ знатоков чеченского</b>\n<i>по количеству верных ответов в /quiz</i>\n\n"
	QuizTopEmptyText      = "Пока никто не набрал очков в /quiz. Стань первым! 🧠"
	WordOfDayHour         = 9 // local hour (container TZ is Europe/Moscow)
	WordOfDayFormat       = "📖 <b>Слово дня</b>\n\n<b>%s</b> — %s\n\n<i>Учите чеченский каждый день! 🇨🇪</i>"
	WotdStatusOnText      = "📖 <b>Слово дня</b>\n\nВы подписаны ✅ — каждый день в 9:00 будете получать новое чеченское слово."
	WotdStatusOffText     = "📖 <b>Слово дня</b>\n\nПодпишитесь, чтобы каждое утро получать новое чеченское слово и пополнять словарный запас."
	WotdSubscribeButton   = "🔔 Подписаться"
	WotdUnsubscribeButton = "🔕 Отписаться"
	WotdSubscribedToast   = "Вы подписались на слово дня! 🔔"
	WotdUnsubscribedToast = "Вы отписались от слова дня"
	DonationMessageFormat = "🌱 Чтобы наш проект мог продолжить работать, вы можете помочь нам"
	DefaultModerationChat = int64(-5204234916)
	BroadcastParseMode         = "html"
	BroadcastSendDelay         = 100 * time.Millisecond
	FreeSpellcheckLimit        = 5
	SubscriptionPriceKopecks   = 10000 // 100 RUB
	SubscriptionPriceFormatted = "100 ₽"
	SubscriptionDuration       = 30 * 24 * time.Hour // 30 days
)

type AI interface {
	SpellCheck(ctx context.Context, text string) (*ai.SpellCheckResult, error)
}

type Business interface {
	Translate(word string) []models.TranslationPairs
	TranslateFormatted(word string) *models.TranslationResult
	SetAIFormatting(enabled bool)
	AIFormattingEnabled() bool
	RandomWordFromAPI(ctx context.Context) (*models.RandomWord, error)
	GenerateQuiz(ctx context.Context) (*models.QuizQuestion, error)
	GrammarFor(ctx context.Context, word string) *models.WordGrammar
}

type Repository interface {
	StoreUser(ctx context.Context, userID int, username string) error
	StoreActivity(ctx context.Context, userID int, activityType models.ActivityType) error
	CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error)
	DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]models.DailyActivity, error)
	MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error)
	ShouldSendDonationMessage(ctx context.Context, userID int) (bool, error)
	StoreDonationMessage(ctx context.Context, userID int) error
	ListUserIDs(ctx context.Context) ([]int64, error)
	MarkUserBlocked(ctx context.Context, userID int64, reason string) error
	MarkUserUnblocked(ctx context.Context, userID int64) error
	ListPendingTranslationPairs(ctx context.Context, limit int) ([]repository.TranslationPair, error)
	ListPendingTranslationPairsByWord(ctx context.Context, cleanWord string, limit int) ([]repository.TranslationPair, error)
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	FindStrictlyApprovedPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	GetPairCleanWords(ctx context.Context, pairID int64) ([]string, error)
	StoreSpellcheckFeedback(ctx context.Context, userID int64, originalText, correctedText, feedback string) error
	GetSpellcheckUsage(ctx context.Context, userID int64, month, year int) (int, error)
	IncrementSpellcheckUsage(ctx context.Context, userID int64, month, year int) error
	HasActiveSubscription(ctx context.Context, userID int64) (bool, error)
	CreateSubscription(ctx context.Context, userID int64, expiresAt time.Time, telegramPaymentID string) error
	RecordMissingWord(ctx context.Context, cleanWord, rawWord string) error
	TopMissingWords(ctx context.Context, limit int) ([]models.MissingWord, error)
	RandomApprovedPair(ctx context.Context) (*models.RandomWord, error)
	CountDictionaryPairs(ctx context.Context) (total int, approved int, err error)
	CountMissingWords(ctx context.Context) (int, error)
	RecordQuizAnswer(ctx context.Context, userID int64, username string, correct bool) error
	GetQuizScore(ctx context.Context, userID int64) (correct int, total int, err error)
	TopQuizScorers(ctx context.Context, limit int) ([]models.QuizScorer, error)
	CountQuizStats(ctx context.Context) (players, totalAnswers, correctAnswers int, err error)
	SetWordOfDaySubscription(ctx context.Context, userID int64, subscribed bool) error
	IsWordOfDaySubscribed(ctx context.Context, userID int64) (bool, error)
	ListWordOfDaySubscribers(ctx context.Context) ([]int64, error)
	CountWordOfDaySubscribers(ctx context.Context) (int, error)
}

type Net struct {
	log               *logrus.Logger
	repo              Repository
	business          Business
	ai                AI
	bot               *tgbotapi.BotAPI
	cache             *cache.Cache
	awaitingBroadcast bool
	pendingBroadcast  *broadcastPayload
}

func NewNet(log *logrus.Logger, repo Repository, bot *tgbotapi.BotAPI, business Business, cache *cache.Cache, aiClient AI) *Net {
	return &Net{
		log:      log,
		repo:     repo,
		bot:      bot,
		business: business,
		ai:       aiClient,
		cache:    cache,
	}
}

func (n *Net) Start(ctx context.Context) {
	n.log.Info("starting service")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := n.bot.GetUpdatesChan(u)

	for update := range updates {
		// Callbacks
		if update.CallbackQuery != nil {
			n.routeCallback(ctx, &update)
			continue
		}

		// Poll answers (group quiz scoring)
		if update.PollAnswer != nil {
			if err := n.HandlePollAnswer(ctx, &update); err != nil {
				n.log.WithError(err).Error("service.HandlePollAnswer")
			}
			continue
		}

		// Pre-checkout query (Telegram Payments)
		if update.PreCheckoutQuery != nil {
			if err := n.HandlePreCheckout(&update); err != nil {
				n.log.WithError(err).Error("service.HandlePreCheckout")
			}
			continue
		}

		// Messages
		if update.Message != nil {
			// Successful payment
			if update.Message.SuccessfulPayment != nil {
				if err := n.HandleSuccessfulPayment(ctx, &update); err != nil {
					n.log.WithError(err).Error("service.HandleSuccessfulPayment")
				}
				continue
			}
			n.routeMessage(ctx, &update)
			continue
		}

		// Inline queries
		if update.InlineQuery != nil && update.InlineQuery.Query != "" {
			n.routeInline(ctx, &update)
			continue
		}
	}
}

func (n *Net) routeCallback(ctx context.Context, update *tgbotapi.Update) {
	data := update.CallbackQuery.Data
	var err error

	switch {
	case strings.HasPrefix(data, "broadcast_"):
		err = n.HandleBroadcastCallback(ctx, update)
	case strings.HasPrefix(data, "more_"):
		err = n.HandleMoreTranslations(ctx, update)
	case strings.HasPrefix(data, "random_"):
		err = n.HandleRandomCallback(ctx, update)
	case strings.HasPrefix(data, "quiz_"):
		err = n.HandleQuizCallback(ctx, update)
	case strings.HasPrefix(data, "wotd_"):
		err = n.HandleWordOfDayCallback(ctx, update)
	case strings.HasPrefix(data, "spell_"):
		err = n.HandleSpellcheckFeedback(ctx, update)
	case strings.HasPrefix(data, "mod_"):
		err = n.HandleModerationCallback(ctx, update)
	}

	if err != nil {
		n.log.WithError(err).WithField("callback", data).Error("callback handler failed")
	}
}

func (n *Net) routeMessage(ctx context.Context, update *tgbotapi.Update) {
	var err error

	switch update.Message.Command() {
	case "start":
		err = n.HandleStart(update)
	case "stats":
		err = n.HandleStats(ctx, update)
	case "missing":
		err = n.HandleMissingWords(ctx, update)
	case "random":
		err = n.HandleRandom(ctx, update.Message.Chat.ID)
	case "quiz":
		err = n.HandleQuiz(ctx, update.Message.Chat)
	case "top":
		err = n.HandleTop(ctx, update.Message.Chat.ID)
	case "wotd":
		err = n.HandleWordOfDay(ctx, update)
	case "moderate":
		err = n.HandleModerate(ctx, update)
	case "check":
		err = n.HandleCheck(ctx, update)
	case "subscribe":
		err = n.HandleSubscribe(ctx, update)
	case "ai":
		n.HandleAIToggle(update.Message)
		return
	case "broadcast":
		err = n.HandleBroadcast(ctx, update)
	case "broadcast_cancel":
		err = n.HandleBroadcastCancel(update)
	default:
		// Spellcheck: message starts with "."
		if strings.HasPrefix(update.Message.Text, ".") && len(update.Message.Text) > 1 {
			update.Message.Text = strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "."))
			if update.Message.Text != "" {
				err = n.HandleCheck(ctx, update)
				if err != nil {
					n.log.WithError(err).Error("service.HandleCheck (dot prefix)")
				}
				return
			}
		}

		if n.isAwaitingBroadcastContent(update) {
			err = n.HandleBroadcastContent(update)
		} else {
			err = n.HandleText(ctx, update)
		}
	}

	if err != nil {
		n.log.
			WithField("user_id", update.Message.From.ID).
			WithField("command", update.Message.Command()).
			WithError(err).
			Error("message handler failed")
	}
}

func (n *Net) routeInline(ctx context.Context, update *tgbotapi.Update) {
	var err error

	if strings.HasPrefix(update.InlineQuery.Query, ". ") && len(update.InlineQuery.Query) > 2 {
		err = n.HandleInlineSpellcheck(ctx, update)
	} else {
		err = n.HandleInline(ctx, update)
	}

	if err != nil {
		n.log.
			WithField("user_id", update.InlineQuery.From.ID).
			WithField("query", update.InlineQuery.Query).
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
		n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: ON"))
	case "off":
		n.business.SetAIFormatting(false)
		n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: OFF"))
	default:
		status := "OFF"
		if n.business.AIFormattingEnabled() {
			status = "ON"
		}
		n.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, "AI formatting: "+status+"\n/ai on | /ai off"))
	}
}

func (n *Net) isBlockedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "bot was blocked") ||
		strings.Contains(errStr, "user is deactivated") ||
		strings.Contains(errStr, "chat not found")
}

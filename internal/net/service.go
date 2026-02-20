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
	MoreTranslationsHelpText = `<i>Чтобы просмотреть все доступные переводы, нажмите на кнопку «Еще» или воспользуйтесь инлайн-режимом: введите @chetoru_bot и слово, которое хотите перевести. Это позволит вам увидеть все варианты.</i>`
	StartMessageText         = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео"
	NoTranslationText        = "К сожалению, нет перевода"
	MoreButtonText           = "Еще (%d)"
	StatsHeaderText          = `
<b>Статистика</b>

Новых пользователей за месяц: %d

Активных пользователей за месяц: %d

Уникальных пользователей на протяжении месяца:
<i>число месяца - кол-во уникальных пользователей - кол-во вызовов бота (включая инлайн)</i>
`
	DailyStatsFormat      = "%d - %d - %d\n"
	DonationMessageFormat = "🌱 Чтобы наш проект мог продолжить работать, вы можете помочь нам"
	DefaultModerationChat = int64(-5204234916)
	BroadcastParseMode    = "html"
	BroadcastSendDelay    = 100 * time.Millisecond
)

type AI interface {
	SpellCheck(ctx context.Context, text string) (*ai.SpellCheckResult, error)
}

type Business interface {
	Translate(word string) []models.TranslationPairs
	TranslateFormatted(word string) *models.TranslationResult
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

		// Messages
		if update.Message != nil {
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
	case "moderate":
		err = n.HandleModerate(ctx, update)
	case "check":
		err = n.HandleCheck(ctx, update)
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

func (n *Net) isBlockedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "bot was blocked") ||
		strings.Contains(errStr, "user is deactivated") ||
		strings.Contains(errStr, "chat not found")
}

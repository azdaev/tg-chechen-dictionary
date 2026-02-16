package net

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"

	"context"
	"fmt"
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

type broadcastPayload struct {
	Text     string
	PhotoID  string
	Caption  string
	HasPhoto bool
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
}

type Net struct {
	log               *logrus.Logger
	repo              Repository
	business          Business
	bot               *tgbotapi.BotAPI
	cache             *cache.Cache
	awaitingBroadcast bool
	pendingBroadcast  *broadcastPayload
}

func NewNet(log *logrus.Logger, repo Repository, bot *tgbotapi.BotAPI, business Business, cache *cache.Cache) *Net {
	return &Net{
		log:      log,
		repo:     repo,
		bot:      bot,
		business: business,
		cache:    cache,
	}
}

func (n *Net) Start(ctx context.Context) {
	n.log.Info("starting service")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := n.bot.GetUpdatesChan(u)

	for update := range updates {
		// Обработка callback запросов
		if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "broadcast_") {
			err := n.HandleBroadcastCallback(ctx, &update)
			if err != nil {
				n.log.WithError(err).Error("service.HandleBroadcastCallback")
			}
			continue
		}
		if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "more_") {
			err := n.HandleMoreTranslations(ctx, &update)
			if err != nil {
				n.log.WithError(err).Error("service.HandleMoreTranslations")
			}
			continue
		}
		if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "mod_") {
			err := n.HandleModerationCallback(ctx, &update)
			if err != nil {
				n.log.WithError(err).Error("service.HandleModerationCallback")
			}
			continue
		}

		// Обработка текстовых сообщений
		if update.Message != nil {
			if update.Message.Command() == "start" {
				err := n.HandleStart(&update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleStart")
				}
				continue
			}

			if update.Message.Command() == "stats" {
				err := n.HandleStats(ctx, &update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleStats")
				}
				continue
			}
			if update.Message.Command() == "moderate" {
				err := n.HandleModerate(ctx, &update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleModerate")
				}
				continue
			}
			if update.Message.Command() == "broadcast" {
				err := n.HandleBroadcast(ctx, &update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleBroadcast")
				}
				continue
			}
			if update.Message.Command() == "broadcast_cancel" {
				err := n.HandleBroadcastCancel(&update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleBroadcastCancel")
				}
				continue
			}

			if n.isAwaitingBroadcastContent(&update) {
				err := n.HandleBroadcastContent(&update)
				if err != nil {
					n.log.
						WithError(err).
						Error("service.HandleBroadcastContent")
				}
				continue
			}

			err := n.HandleText(ctx, &update)
			if err != nil {
				n.log.
					WithField("user_id", update.Message.From.ID).
					WithField("username", update.Message.From.UserName).
					WithField("message", update.Message.Text).
					WithError(err).
					Error("service.HandleText")
			}
			continue
		}

		// Обработка inline запросов
		if update.InlineQuery != nil && update.InlineQuery.Query != "" {
			err := n.HandleInline(ctx, &update)
			if err != nil {
				n.log.
					WithField("user_id", update.InlineQuery.From.ID).
					WithField("username", update.InlineQuery.From.UserName).
					WithField("message", update.InlineQuery.Query).
					WithError(err).
					Error("service.HandleInline")
			}
			continue
		}
	}
}

func (n *Net) HandleBroadcast(ctx context.Context, update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	n.awaitingBroadcast = true
	n.pendingBroadcast = nil
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Отправьте текст или фото с подписью. Я покажу превью перед рассылкой.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastCancel(update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	n.awaitingBroadcast = false
	n.pendingBroadcast = nil
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Рассылка отменена.")
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) HandleBroadcastContent(update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	payload, err := buildBroadcastPayload(update.Message)
	if err != nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
		_, sendErr := n.bot.Send(msg)
		if sendErr != nil {
			return sendErr
		}
		return nil
	}

	n.awaitingBroadcast = false
	n.pendingBroadcast = payload
	preview, err := n.sendBroadcastPreview(update.Message.Chat.ID, payload)
	if err != nil {
		return err
	}
	_, err = n.bot.Send(preview)
	return err
}

func (n *Net) HandleBroadcastCallback(ctx context.Context, update *tgbotapi.Update) error {
	if update.CallbackQuery == nil {
		return nil
	}

	if !n.isAdmin(update.CallbackQuery.From.ID) {
		return nil
	}

	data := update.CallbackQuery.Data
	switch data {
	case "broadcast_send":
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Отправляю")
		if _, err := n.bot.Request(callback); err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}

		go func() {
			if err := n.sendBroadcast(ctx, update); err != nil {
				n.log.WithError(err).Error("service.sendBroadcast")
			}
		}()
		return nil
	case "broadcast_cancel":
		n.pendingBroadcast = nil
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Отменено")
		_, err := n.bot.Request(callback)
		if err != nil {
			return fmt.Errorf("bot.Request: %w", err)
		}
		msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Рассылка отменена.")
		_, err = n.bot.Send(msg)
		return err
	default:
		return nil
	}
}

func (n *Net) sendBroadcast(ctx context.Context, update *tgbotapi.Update) error {
	payload := n.pendingBroadcast
	if payload == nil {
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "Нет данных для рассылки")
		_, err := n.bot.Request(callback)
		return err
	}

	userIDs, err := n.repo.ListUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("repo.ListUserIDs: %w", err)
	}

	sent := 0
	failed := 0
	blocked := 0
	for _, userID := range userIDs {
		sendErr := n.sendBroadcastPayload(userID, payload)
		if sendErr != nil {
			failed++
			if n.isBlockedError(sendErr) {
				if err := n.repo.MarkUserBlocked(ctx, userID, sendErr.Error()); err != nil {
					n.log.WithError(err).WithField("user_id", userID).Warn("failed to mark user blocked")
				} else {
					blocked++
				}
			}
			n.log.WithError(sendErr).WithField("user_id", userID).Warn("broadcast send failed")
		} else {
			sent++
		}
		time.Sleep(BroadcastSendDelay)
	}

	n.pendingBroadcast = nil

	summary := fmt.Sprintf("Рассылка завершена. Всего: %d, отправлено: %d, ошибки: %d, заблокировано: %d", len(userIDs), sent, failed, blocked)
	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, summary)
	_, err = n.bot.Send(msg)
	return err
}

func (n *Net) sendBroadcastPreview(chatID int64, payload *broadcastPayload) (tgbotapi.Chattable, error) {
	if payload.HasPhoto {
		preview := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(payload.PhotoID))
		preview.Caption = payload.Caption
		preview.ParseMode = BroadcastParseMode
		preview.ReplyMarkup = broadcastPreviewKeyboard()
		return preview, nil
	}

	preview := tgbotapi.NewMessage(chatID, payload.Text)
	preview.ParseMode = BroadcastParseMode
	preview.ReplyMarkup = broadcastPreviewKeyboard()
	return preview, nil
}

func (n *Net) sendBroadcastPayload(chatID int64, payload *broadcastPayload) error {
	if payload.HasPhoto {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(payload.PhotoID))
		msg.Caption = payload.Caption
		msg.ParseMode = BroadcastParseMode
		_, err := n.bot.Send(msg)
		return err
	}

	msg := tgbotapi.NewMessage(chatID, payload.Text)
	msg.ParseMode = BroadcastParseMode
	_, err := n.bot.Send(msg)
	return err
}

func (n *Net) isAwaitingBroadcastContent(update *tgbotapi.Update) bool {
	if update.Message == nil {
		return false
	}

	if !n.awaitingBroadcast {
		return false
	}

	return n.isAdmin(update.Message.From.ID)
}

func (n *Net) isAdmin(userID int64) bool {
	return strconv.Itoa(int(userID)) == os.Getenv("TG_ADMIN_ID")
}

func buildBroadcastPayload(message *tgbotapi.Message) (*broadcastPayload, error) {
	if message == nil {
		return nil, fmt.Errorf("Нет сообщения для рассылки")
	}

	if message.Photo != nil && len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		caption := message.Caption
		return &broadcastPayload{
			PhotoID:  photo.FileID,
			Caption:  caption,
			HasPhoto: true,
		}, nil
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil, fmt.Errorf("Нужен текст или фото с подписью для рассылки")
	}

	return &broadcastPayload{
		Text: text,
	}, nil
}

func broadcastPreviewKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Отправить", "broadcast_send"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "broadcast_cancel"),
		),
	)
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

func (n *Net) HandleModerate(ctx context.Context, update *tgbotapi.Update) error {
	if strconv.Itoa(int(update.Message.From.ID)) != os.Getenv("TG_ADMIN_ID") {
		return nil
	}

	limit := 20
	args := strings.Fields(update.Message.CommandArguments())
	if len(args) > 0 {
		if parsed, err := strconv.Atoi(args[0]); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	pairs, err := n.repo.ListPendingTranslationPairs(ctx, limit)
	if err != nil {
		return fmt.Errorf("repo.ListPendingTranslationPairs: %w", err)
	}
	if len(pairs) == 0 {
		_, err = n.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Нет новых слов для модерации."))
		return err
	}

	modChatID := moderationChatID()
	for _, pair := range pairs {
		text := formatModerationMessage(pair)
		msg := tgbotapi.NewMessage(modChatID, text)
		
		// Show simplified buttons: only AI and Delete
		if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✅ Принять AI", fmt.Sprintf("mod_ai_%d", pair.ID)),
					tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
				),
			)
		} else {
			// If AI formatting is not ready, show only delete option
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
				),
			)
		}
		if _, err := n.bot.Send(msg); err != nil {
			n.log.WithError(err).WithField("pair_id", pair.ID).Warn("failed to send moderation message")
		}
	}

	_, err = n.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Отправлено на модерацию: %d", len(pairs))))
	return err
}

func (n *Net) HandleModerationCallback(ctx context.Context, update *tgbotapi.Update) error {
	data := update.CallbackQuery.Data
	parts := strings.Split(data, "_")
	if len(parts) != 3 {
		return fmt.Errorf("invalid moderation callback format")
	}

	action := parts[1]
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid moderation id: %w", err)
	}

	var status string
	var choice string

	switch action {
	case "ai":
		// Accept AI formatting
		status = "✅ Принято (AI)"
		choice = "ai"
		if err := n.repo.SetTranslationPairFormattingChoice(ctx, id, choice); err != nil {
			return fmt.Errorf("repo.SetTranslationPairFormattingChoice: %w", err)
		}
	case "delete":
		// Delete (mark as deleted)
		status = "🗑 Удалено"
		choice = "deleted"
		if err := n.repo.SetTranslationPairFormattingChoice(ctx, id, choice); err != nil {
			return fmt.Errorf("repo.SetTranslationPairFormattingChoice: %w", err)
		}
	default:
		return fmt.Errorf("unknown moderation action: %s", action)
	}

	// Invalidate cache for this word
	// We need to extract the clean word from the message or get it from database
	// For simplicity, let's get pairs that contain this ID and invalidate their clean words
	go func() {
		// This is a background task to invalidate cache
		// We could improve this by passing the cleanWord directly, but for now this works
		n.invalidateCacheForPair(ctx, id)
	}()

	edited := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, status+"\n\n"+update.CallbackQuery.Message.Text)
	if _, err := n.bot.Send(edited); err != nil {
		n.log.WithError(err).Warn("failed to edit moderation message")
	}

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, status)
	_, err = n.bot.Request(callback)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}

	return nil
}

// invalidateCacheForPair invalidates cache for a specific pair
func (n *Net) invalidateCacheForPair(ctx context.Context, pairID int64) {
	if n.cache == nil {
		return
	}

	cleanWords, err := n.repo.GetPairCleanWords(ctx, pairID)
	if err != nil {
		n.log.WithError(err).Warn("failed to get clean words for cache invalidation")
		return
	}

	for _, word := range cleanWords {
		if word == "" {
			continue
		}
		_ = n.cache.Delete(ctx, word)
		_ = n.cache.Delete(ctx, "formatted_"+word)
	}
}

func moderationChatID() int64 {
	if val := os.Getenv("TG_MOD_CHAT_ID"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil {
			return id
		}
	}
	return DefaultModerationChat
}

func formatModerationMessage(pair repository.TranslationPair) string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("ID: %d\n", pair.ID))
	sb.WriteString(fmt.Sprintf("%s → %s\n", 
		pair.OriginalClean+" ("+pair.OriginalLang+")",
		pair.TranslationClean+" ("+pair.TranslationLang+")"))
	sb.WriteString(fmt.Sprintf("raw: %s → %s\n", pair.OriginalRaw, pair.TranslationRaw))
	sb.WriteString(fmt.Sprintf("source: %s\n\n", pair.Source))
	
	// Legacy formatting (always show)
	legacyFormat := tools.FormatTranslationLite(
		fmt.Sprintf("**%s** - %s", pair.OriginalRaw, pair.TranslationRaw),
		pair.OriginalRaw,
	)
	sb.WriteString("📋 Legacy:\n")
	sb.WriteString(legacyFormat)
	sb.WriteString("\n\n")
	
	// AI formatting (if available)
	if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
		sb.WriteString("✨ AI:\n")
		sb.WriteString(pair.FormattedAI.String)
	} else {
		sb.WriteString("✨ AI: (форматируется...)")
	}
	
	return sb.String()
}

func (n *Net) HandleText(ctx context.Context, update *tgbotapi.Update) error {
	loaderMessage, err := n.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "⌛️"))
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	defer func() {
		n.bot.Send(
			tgbotapi.NewDeleteMessage(update.Message.Chat.ID, loaderMessage.MessageID),
		)
	}()

	n.bot.Send(tgbotapi.NewChatAction(update.Message.Chat.ID, tgbotapi.ChatTyping))

	err = n.repo.StoreUser(ctx, int(update.Message.From.ID), update.Message.From.UserName)
	if err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	if err := n.repo.MarkUserUnblocked(ctx, update.Message.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", update.Message.From.ID).Warn("failed to unblock user")
	}

	err = n.repo.StoreActivity(ctx, int(update.Message.From.ID), models.ActivityTypeText)
	if err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	m := update.Message
	result := n.business.TranslateFormatted(m.Text)
	if len(result.Pairs) == 0 {
		_, err = n.bot.Send(tgbotapi.NewMessage(m.Chat.ID, NoTranslationText))
		if err != nil {
			return fmt.Errorf("bot.Send: %w", err)
		}
		return nil
	}

	// Показываем только первые 4 перевода для кнопки "Еще"
	firstTranslations := result.Pairs
	if len(result.Pairs) > MaxTranslations {
		firstTranslations = result.Pairs[:MaxTranslations]
	}

	// Используем кэшированный отформатированный текст для первых переводов
	var messageText string
	if len(result.Pairs) <= MaxTranslations {
		// Все переводы помещаются - используем полный отформатированный текст
		messageText = result.Formatted
	} else {
		// Нужно показать только первые 4 - форматируем их отдельно
		messageText = formatTranslations(firstTranslations)
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, messageText)
	msg.ParseMode = "html"

	if len(result.Pairs) > MaxTranslations {
		remainingCount := len(result.Pairs) - MaxTranslations
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, remainingCount),
					fmt.Sprintf("more_%s_4", m.Text),
				),
			),
		)
		msg.ReplyMarkup = keyboard

		msg.Text += "\n\n" + MoreTranslationsHelpText
	}

	_, err = n.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	// Auto-moderation is now triggered via Business.onPairReady callback
	// after AI formatting completes (see main.go)

	// Check if we should send a donation message
	shouldSend, err := n.repo.ShouldSendDonationMessage(ctx, int(update.Message.From.ID))
	if err != nil {
		return fmt.Errorf("failed to check donation message status: %w", err)
	}

	if shouldSend {
		donationMsg := tgbotapi.NewMessage(m.Chat.ID, DonationMessageFormat)
		donationMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("🚀 Поддержать нас", os.Getenv("DONATION_LINK")),
			),
		)
		_, err = n.bot.Send(donationMsg)
		if err != nil {
			return fmt.Errorf("failed to send donation message: %w", err)
		}

		err = n.repo.StoreDonationMessage(ctx, int(update.Message.From.ID))
		if err != nil {
			return fmt.Errorf("failed to store donation message: %w", err)
		}
	}

	return nil
}

// SendAutoModeration sends pending pairs for a word to moderation chat.
// Called from Business.onPairReady callback after AI formatting completes.
func (n *Net) SendAutoModeration(ctx context.Context, word string) {
	cleanWord := tools.NormalizeSearch(word)
	if cleanWord == "" {
		return
	}

	approved, err := n.repo.FindStrictlyApprovedPairs(ctx, cleanWord, 1)
	if err != nil || len(approved) > 0 {
		return
	}

	pairs, err := n.repo.ListPendingTranslationPairsByWord(ctx, cleanWord, 20)
	if err != nil || len(pairs) == 0 {
		return
	}

	modChatID := moderationChatID()
	for _, pair := range pairs {
		text := formatModerationMessage(pair)
		msg := tgbotapi.NewMessage(modChatID, text)
		
		// Show simplified buttons: only AI and Delete
		if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("✅ Принять AI", fmt.Sprintf("mod_ai_%d", pair.ID)),
					tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
				),
			)
		} else {
			// If AI formatting is not ready, show only delete option
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
				),
			)
		}
		if _, err := n.bot.Send(msg); err != nil {
			n.log.WithError(err).WithField("pair_id", pair.ID).Warn("failed to send moderation message")
		}
	}
}

func (n *Net) HandleInline(ctx context.Context, update *tgbotapi.Update) error {
	translations := n.business.Translate(update.InlineQuery.Query)

	articles := make([]interface{}, len(translations))

	for i := range articles {
		article := tgbotapi.NewInlineQueryResultArticle(update.InlineQuery.ID+strconv.Itoa(i), tools.Clean(translations[i].Original), "")
		article.Description = tools.Clean(translations[i].Translate)
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text:      fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate),
			ParseMode: "html",
		}

		articles[i] = article
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: update.InlineQuery.ID,
		IsPersonal:    true,
		CacheTime:     0,
		Results:       articles,
	}

	resp, err := n.bot.Request(inlineConf)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}
	if !resp.Ok {
		return fmt.Errorf("bot.Request: %s", resp.Description)
	}

	err = n.repo.StoreUser(ctx, int(update.InlineQuery.From.ID), update.InlineQuery.From.UserName)
	if err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	if err := n.repo.MarkUserUnblocked(ctx, update.InlineQuery.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", update.InlineQuery.From.ID).Warn("failed to unblock user")
	}

	err = n.repo.StoreActivity(ctx, int(update.InlineQuery.From.ID), models.ActivityTypeInline)
	if err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	return nil
}

func (n *Net) HandleStart(update *tgbotapi.Update) error {
	video := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FilePath(PathInlineVideo))
	video.Caption = StartMessageText

	_, err := n.bot.Send(video)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	return nil
}

func (n *Net) HandleStats(ctx context.Context, update *tgbotapi.Update) error {
	if strconv.Itoa(int(update.Message.From.ID)) != os.Getenv("TG_ADMIN_ID") {
		return nil
	}

	day := time.Now().Day()
	month := int(time.Now().Month())
	year := time.Now().Year()
	newMonthlyUsers, err := n.repo.CountNewMonthlyUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.CountNewMonthlyUsers: %w", err)
	}

	dailyActiveUsersLastMonth, err := n.repo.DailyActiveUsersInMonth(ctx, month, year, day)
	if err != nil {
		return fmt.Errorf("repo.DailyActiveUsersInMonth: %w", err)
	}

	monthlyActiveUsers, err := n.repo.MonthlyActiveUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.MonthlyActiveUsers: %w", err)
	}

	msg := tgbotapi.NewMessage(
		update.Message.Chat.ID,
		statsMessageText(newMonthlyUsers, monthlyActiveUsers, dailyActiveUsersLastMonth),
	)
	msg.ParseMode = "html"

	_, err = n.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	return nil
}

func statsMessageText(newMonthlyUsers int, monthlyActiveUsers int, dailyActivityInMonth []models.DailyActivity) string {
	messageText := fmt.Sprintf(StatsHeaderText, newMonthlyUsers, monthlyActiveUsers)

	for i, activity := range dailyActivityInMonth {
		day := i + 1
		messageText += fmt.Sprintf(DailyStatsFormat, day, activity.ActiveUsers, activity.Calls)
	}

	return messageText
}

func (n *Net) HandleMoreTranslations(ctx context.Context, update *tgbotapi.Update) error {
	parts := strings.Split(update.CallbackQuery.Data, "_")
	if len(parts) != 3 {
		return fmt.Errorf("invalid callback data format")
	}

	word := parts[1]                    // слово, которое нужно перевести
	offset, _ := strconv.Atoi(parts[2]) // номер первого перевода, который нужно показать

	translations := n.business.Translate(word)
	if len(translations) == 0 {
		_, err := n.bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, NoTranslationText))
		if err != nil {
			return fmt.Errorf("bot.Send: %w", err)
		}
		return nil
	}

	// Получаем следующие 4 перевода
	end := min(offset+4, len(translations))
	nextTranslations := translations[offset:end]

	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, formatTranslations(nextTranslations))
	msg.ParseMode = "html"

	// Если есть еще переводы, добавляем новую кнопку "еще"
	if end < len(translations) {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, len(translations)-end),
					fmt.Sprintf("more_%s_%d", word, end),
				),
			),
		)
		msg.ReplyMarkup = keyboard
	}

	_, err := n.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	// Отвечаем на callback query, чтобы убрать "часики" с кнопки
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	_, err = n.bot.Request(callback)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}

	return nil
}

// Вспомогательная функция для форматирования переводов
func formatTranslations(translations []models.TranslationPairs) string {
	var result string
	for _, t := range translations {
		// Проверяем, должны ли мы использовать AI форматирование
		if t.FormattedChosen == "ai" && t.FormattedAI != "" {
			result += t.FormattedAI + "\n\n"
			continue
		}

		// Используем legacy форматирование
		// Определяем сложность перевода: нумерация (1), 2)) или тильды (~)
		isComplexTranslation := strings.Contains(t.Translate, "1)") ||
			strings.Contains(t.Translate, "2)") ||
			strings.Contains(t.Translate, "~") ||
			strings.Contains(t.Original, "1)") ||
			strings.Contains(t.Original, "2)") ||
			strings.Contains(t.Original, "~")

		if isComplexTranslation {
			// Определяем, какое поле содержит сложный перевод
			if strings.Contains(t.Translate, "1)") || strings.Contains(t.Translate, "2)") || strings.Contains(t.Translate, "~") {
				// Создаем словарную статью в нужном формате
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Original, t.Translate)
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Original)
				result += formatted + "\n\n"
			} else if strings.Contains(t.Original, "1)") || strings.Contains(t.Original, "2)") || strings.Contains(t.Original, "~") {
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Translate, t.Original)
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Translate)
				result += formatted + "\n\n"
			}
		} else {
			// Обычное форматирование для простых переводов
			result += fmt.Sprintf("%s — %s\n\n", t.Original, tools.Clean(t.Translate))
		}
	}
	return result
}

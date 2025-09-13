package net

import (
	"chetoru/internal/models"
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
)

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
}

type Net struct {
	log      *logrus.Logger
	repo     Repository
	business Business
	bot      *tgbotapi.BotAPI
}

func NewNet(log *logrus.Logger, repo Repository, bot *tgbotapi.BotAPI, business Business) *Net {
	return &Net{
		log:      log,
		repo:     repo,
		bot:      bot,
		business: business,
	}
}

func (n *Net) Start(ctx context.Context) {
	n.log.Info("starting service")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := n.bot.GetUpdatesChan(u)

	for update := range updates {
		// Обработка callback запросов
		if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "more_") {
			err := n.HandleMoreTranslations(ctx, &update)
			if err != nil {
				n.log.WithError(err).Error("service.HandleMoreTranslations")
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
		// Используем новое форматирование если перевод содержит словарную статью
		if strings.Contains(t.Original, "**") || strings.Contains(t.Translate, "**") {
			// Определяем, какое поле содержит словарную статью
			if strings.Contains(t.Translate, "**") {
				formatted := tools.FormatTranslation(t.Translate)
				result += formatted + "\n\n"
			} else if strings.Contains(t.Original, "**") {
				formatted := tools.FormatTranslation(t.Original)
				result += formatted + "\n\n"
			}
		} else {
			// Обычное форматирование для простых переводов
			result += fmt.Sprintf("<b>%s</b> - %s\n\n", t.Original, t.Translate)
		}
	}
	return result
}

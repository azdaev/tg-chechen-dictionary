package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleText(ctx context.Context, update *tgbotapi.Update) error {
	loaderMessage, err := n.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "⌛️"))
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	defer func() {
		n.bot.Send(tgbotapi.NewDeleteMessage(update.Message.Chat.ID, loaderMessage.MessageID))
	}()

	n.bot.Send(tgbotapi.NewChatAction(update.Message.Chat.ID, tgbotapi.ChatTyping))

	if err := n.repo.StoreUser(ctx, int(update.Message.From.ID), update.Message.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	if err := n.repo.MarkUserUnblocked(ctx, update.Message.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", update.Message.From.ID).Warn("failed to unblock user")
	}

	if err := n.repo.StoreActivity(ctx, int(update.Message.From.ID), models.ActivityTypeText); err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	m := update.Message
	result := n.business.TranslateFormatted(m.Text)
	if len(result.Pairs) == 0 {
		_, err = n.bot.Send(tgbotapi.NewMessage(m.Chat.ID, NoTranslationText))
		return err
	}

	firstTranslations := result.Pairs
	if len(result.Pairs) > MaxTranslations {
		firstTranslations = result.Pairs[:MaxTranslations]
	}

	var messageText string
	if len(result.Pairs) <= MaxTranslations {
		messageText = result.Formatted
	} else {
		messageText = formatTranslations(firstTranslations)
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, messageText)
	msg.ParseMode = "html"

	if len(result.Pairs) > MaxTranslations {
		remainingCount := len(result.Pairs) - MaxTranslations
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, remainingCount),
					fmt.Sprintf("more_%s_4", m.Text),
				),
			),
		)
		msg.Text += "\n\n" + MoreTranslationsHelpText
	}

	if _, err = n.bot.Send(msg); err != nil {
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
		if _, err = n.bot.Send(donationMsg); err != nil {
			return fmt.Errorf("failed to send donation message: %w", err)
		}
		if err = n.repo.StoreDonationMessage(ctx, int(update.Message.From.ID)); err != nil {
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

	if err := n.repo.StoreUser(ctx, int(update.InlineQuery.From.ID), update.InlineQuery.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}
	if err := n.repo.MarkUserUnblocked(ctx, update.InlineQuery.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", update.InlineQuery.From.ID).Warn("failed to unblock user")
	}
	if err := n.repo.StoreActivity(ctx, int(update.InlineQuery.From.ID), models.ActivityTypeInline); err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	return nil
}

func (n *Net) HandleMoreTranslations(ctx context.Context, update *tgbotapi.Update) error {
	parts := strings.Split(update.CallbackQuery.Data, "_")
	if len(parts) != 3 {
		return fmt.Errorf("invalid callback data format")
	}

	word := parts[1]
	offset, _ := strconv.Atoi(parts[2])

	translations := n.business.Translate(word)
	if len(translations) == 0 {
		_, err := n.bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, NoTranslationText))
		return err
	}

	end := min(offset+4, len(translations))
	nextTranslations := translations[offset:end]

	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, formatTranslations(nextTranslations))
	msg.ParseMode = "html"

	if end < len(translations) {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, len(translations)-end),
					fmt.Sprintf("more_%s_%d", word, end),
				),
			),
		)
	}

	if _, err := n.bot.Send(msg); err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	_, err := n.bot.Request(callback)
	return err
}

func formatTranslations(translations []models.TranslationPairs) string {
	var result string
	for _, t := range translations {
		if t.FormattedChosen == "ai" && t.FormattedAI != "" {
			result += t.FormattedAI + "\n\n"
			continue
		}

		isComplexTranslation := strings.Contains(t.Translate, "1)") ||
			strings.Contains(t.Translate, "2)") ||
			strings.Contains(t.Translate, "~") ||
			strings.Contains(t.Original, "1)") ||
			strings.Contains(t.Original, "2)") ||
			strings.Contains(t.Original, "~")

		if isComplexTranslation {
			if strings.Contains(t.Translate, "1)") || strings.Contains(t.Translate, "2)") || strings.Contains(t.Translate, "~") {
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Original, t.Translate)
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Original)
				result += formatted + "\n\n"
			} else if strings.Contains(t.Original, "1)") || strings.Contains(t.Original, "2)") || strings.Contains(t.Original, "~") {
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Translate, t.Original)
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Translate)
				result += formatted + "\n\n"
			}
		} else {
			result += fmt.Sprintf("%s — %s\n\n", t.Original, tools.Clean(t.Translate))
		}
	}
	return result
}

package net

import (
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleModerate(ctx context.Context, update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
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
		msg.ReplyMarkup = moderationKeyboard(pair)
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

	var status, choice string
	switch action {
	case "ai":
		status, choice = "✅ Принято (AI)", "ai"
	case "delete":
		status, choice = "🗑 Удалено", "deleted"
	default:
		return fmt.Errorf("unknown moderation action: %s", action)
	}

	if err := n.repo.SetTranslationPairFormattingChoice(ctx, id, choice); err != nil {
		return fmt.Errorf("repo.SetTranslationPairFormattingChoice: %w", err)
	}

	go n.invalidateCacheForPair(ctx, id)

	edited := tgbotapi.NewEditMessageText(
		update.CallbackQuery.Message.Chat.ID,
		update.CallbackQuery.Message.MessageID,
		status+"\n\n"+update.CallbackQuery.Message.Text,
	)
	if _, err := n.bot.Send(edited); err != nil {
		n.log.WithError(err).Warn("failed to edit moderation message")
	}

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, status)
	_, err = n.bot.Request(callback)
	return err
}

// SendAutoModeration sends pending pairs for a word to moderation chat.
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
		msg.ReplyMarkup = moderationKeyboard(pair)
		if _, err := n.bot.Send(msg); err != nil {
			n.log.WithError(err).WithField("pair_id", pair.ID).Warn("failed to send moderation message")
		}
	}
}

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

func moderationKeyboard(pair repository.TranslationPair) tgbotapi.InlineKeyboardMarkup {
	if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Принять AI", fmt.Sprintf("mod_ai_%d", pair.ID)),
				tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
			),
		)
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", fmt.Sprintf("mod_delete_%d", pair.ID)),
		),
	)
}

func formatModerationMessage(pair repository.TranslationPair) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("ID: %d\n", pair.ID))
	sb.WriteString(fmt.Sprintf("%s → %s\n",
		pair.OriginalClean+" ("+pair.OriginalLang+")",
		pair.TranslationClean+" ("+pair.TranslationLang+")"))
	sb.WriteString(fmt.Sprintf("raw: %s → %s\n", pair.OriginalRaw, pair.TranslationRaw))
	sb.WriteString(fmt.Sprintf("source: %s\n\n", pair.Source))

	legacyFormat := tools.FormatTranslationLite(
		fmt.Sprintf("**%s** - %s", pair.OriginalRaw, pair.TranslationRaw),
		pair.OriginalRaw,
	)
	sb.WriteString("📋 Legacy:\n")
	sb.WriteString(legacyFormat)
	sb.WriteString("\n\n")

	if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
		sb.WriteString("✨ AI:\n")
		sb.WriteString(pair.FormattedAI.String)
	} else {
		sb.WriteString("✨ AI: (форматируется...)")
	}

	return sb.String()
}

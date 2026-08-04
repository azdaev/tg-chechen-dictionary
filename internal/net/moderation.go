package net

import (
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleModerate(ctx context.Context, m *tgbotapi.Message) error {
	if !n.isAdmin(m.From.ID) {
		return nil
	}

	limit := 20
	args := strings.Fields(m.CommandArguments())
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
		_, err = n.send(tgbotapi.NewMessage(m.Chat.ID, "Нет новых слов для модерации."))
		return err
	}

	modChatID := moderationChatID()
	for _, pair := range pairs {
		text := formatModerationMessage(pair)
		msg := tgbotapi.NewMessage(modChatID, text)
		msg.ReplyMarkup = moderationKeyboard(pair)
		if _, err := n.send(msg); err != nil {
			n.log.WithError(err).WithField("pair_id", pair.ID).Warn("failed to send moderation message")
		}
	}

	_, err = n.send(tgbotapi.NewMessage(m.Chat.ID, fmt.Sprintf("Отправлено на модерацию: %d", len(pairs))))
	return err
}

func (n *Net) HandleModerationCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	// callback_data comes from the client, not from the keyboard we drew: a
	// custom client can send any string, and every /quiz or /random card gives
	// it a message to attach to. Without this the handler wrote straight to the
	// dictionary — «mod_delete_<id>» over a walk of the integer ids emptied the
	// local table for everyone. Chat.ID is stamped by Telegram, so requiring the
	// press to come from the moderation chat is the guard the data cannot forge,
	// and it keeps the whole chat moderating rather than only TG_ADMIN_ID.
	if cq.Message == nil || cq.Message.Chat.ID != moderationChatID() {
		return nil
	}

	data := cq.Data
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

	n.bg.Go(func() { n.invalidateCacheForPair(ctx, id) })

	edited := tgbotapi.NewEditMessageText(
		cq.Message.Chat.ID,
		cq.Message.MessageID,
		status+"\n\n"+cq.Message.Text,
	)
	if _, err := n.send(edited); err != nil {
		n.log.WithError(err).Warn("failed to edit moderation message")
	}

	callback := tgbotapi.NewCallback(cq.ID, status)
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
		if _, err := n.send(msg); err != nil {
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
		_ = n.cache.DeleteTranslation(ctx, word)
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

	fmt.Fprintf(&sb, "ID: %d\n", pair.ID)
	fmt.Fprintf(&sb, "%s → %s\n",
		pair.OriginalClean+" ("+pair.OriginalLang+")",
		pair.TranslationClean+" ("+pair.TranslationLang+")")
	fmt.Fprintf(&sb, "raw: %s → %s\n", pair.OriginalRaw, pair.TranslationRaw)
	fmt.Fprintf(&sb, "source: %s\n\n", pair.Source)

	// What the bot would actually send for this pair. The preview used to render
	// with the parser the bot retired, so the moderator was choosing between the
	// AI rendering and a format no user has seen since — and could reject one
	// that reads better than what ships.
	current := tools.FormatCard(pair.OriginalRaw, []models.TranslationPairs{{
		Original:      pair.OriginalRaw,
		Translate:     pair.TranslationRaw,
		OriginalLang:  pair.OriginalLang,
		TranslateLang: pair.TranslationLang,
		Rate:          pair.Rate,
		EntryType:     pair.EntryType,
		Subtype:       pair.Subtype,
		EntryIndex:    pair.EntryIndex,
		Notes:         pair.EntryNotes,
	}})
	if current == "" {
		current = tools.FormatPairs([]models.TranslationPairs{{
			Original: pair.OriginalRaw, Translate: pair.TranslationRaw,
			OriginalLang: pair.OriginalLang, TranslateLang: pair.TranslationLang,
		}})
	}
	sb.WriteString("📋 Сейчас:\n")
	sb.WriteString(current)
	sb.WriteString("\n\n")

	if pair.FormattedAI.Valid && pair.FormattedAI.String != "" {
		sb.WriteString("✨ AI:\n")
		sb.WriteString(pair.FormattedAI.String)
	} else {
		sb.WriteString("✨ AI: (форматируется...)")
	}

	return sb.String()
}

package net

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleCheck(ctx context.Context, m *tgbotapi.Message) error {
	// Try command arguments first, then raw message text (for dot-prefix mode)
	text := strings.TrimSpace(m.CommandArguments())
	if text == "" {
		text = strings.TrimSpace(m.Text)
	}
	if text == "" {
		msg := tgbotapi.NewMessage(m.Chat.ID,
			"Использование: /check <текст на чеченском>\n\nПример: /check дала безам бу хьо\n\nИли просто начни сообщение с точки:\n.дала безам бу хьо")
		_, err := n.bot.Send(msg)
		return err
	}

	if n.ai == nil {
		msg := tgbotapi.NewMessage(m.Chat.ID, "⚠️ Проверка орфографии временно недоступна")
		_, err := n.bot.Send(msg)
		return err
	}

	n.bot.Send(tgbotapi.NewChatAction(m.Chat.ID, tgbotapi.ChatTyping))

	result, err := n.ai.SpellCheck(ctx, text)
	if err != nil {
		n.log.WithError(err).Error("ai.SpellCheck")
		msg := tgbotapi.NewMessage(m.Chat.ID, "⚠️ Не удалось проверить текст, попробуйте позже")
		_, sendErr := n.bot.Send(msg)
		return sendErr
	}

	if result.NoErrors {
		msg := tgbotapi.NewMessage(m.Chat.ID, "✅ Ошибок не найдено")
		msg.ReplyToMessageID = m.MessageID
		_, err = n.bot.Send(msg)
		return err
	}

	// Format response
	var responseText string
	if result.Corrected != "" {
		responseText = "✏️ " + result.Corrected
	}
	if idx := strings.Index(result.Explanation, "CHANGES:"); idx != -1 {
		changes := strings.TrimSpace(result.Explanation[idx+len("CHANGES:"):])
		if changes != "" {
			responseText += "\n\n📝 Изменения:\n" + changes
		}
	}
	if responseText == "" {
		responseText = result.Explanation
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, responseText)
	msg.ReplyToMessageID = m.MessageID

	if result.Corrected != "" {
		msg.ReplyMarkup = spellcheckFeedbackKeyboard(text, result.Corrected)
	}

	_, err = n.bot.Send(msg)
	return err
}

// spellcheckDebounceDelay is how long an inline spellcheck query must stay the
// user's latest before it is processed. Telegram fires inline queries while
// the user is still typing; without settling, every intermediate prefix would
// cost an AI call and burn a unit of the free quota.
const spellcheckDebounceDelay = 1500 * time.Millisecond

func (n *Net) HandleInlineSpellcheck(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	if n.ai == nil {
		return nil
	}

	n.noteInlineSpellQuery(iq.From.ID, iq.ID)
	// AfterFunc runs outside the update dispatcher, so a debouncing typer never
	// occupies one of its slots.
	time.AfterFunc(spellcheckDebounceDelay, func() {
		defer func() {
			if r := recover(); r != nil {
				n.log.WithField("panic", r).Error("inline spellcheck panicked")
			}
		}()
		if !n.isLatestInlineSpellQuery(iq.From.ID, iq.ID) {
			return // superseded — the user kept typing
		}
		if err := n.runInlineSpellcheck(ctx, iq); err != nil {
			n.log.WithError(err).WithField("user_id", iq.From.ID).Error("inline spellcheck failed")
		}
	})
	return nil
}

func (n *Net) noteInlineSpellQuery(userID int64, queryID string) {
	n.inlineSpellMu.Lock()
	defer n.inlineSpellMu.Unlock()
	n.inlineSpellLatest[userID] = queryID
}

func (n *Net) isLatestInlineSpellQuery(userID int64, queryID string) bool {
	n.inlineSpellMu.Lock()
	defer n.inlineSpellMu.Unlock()
	if n.inlineSpellLatest[userID] != queryID {
		return false
	}
	delete(n.inlineSpellLatest, userID) // settled; stop tracking
	return true
}

func (n *Net) runInlineSpellcheck(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	text := strings.TrimPrefix(iq.Query, ". ")

	// Check usage limits
	allowed, err := n.canUseSpellcheck(ctx, iq.From.ID)
	if err != nil {
		n.log.WithError(err).Error("canUseSpellcheck inline")
	}
	if !allowed {
		article := tgbotapi.NewInlineQueryResultArticle(
			iq.ID+"_limit",
			fmt.Sprintf("🔒 Лимит исчерпан (%d/мес)", FreeSpellcheckLimit),
			"",
		)
		article.Description = fmt.Sprintf("Подписка %s/мес — отправьте боту /subscribe", SubscriptionPriceFormatted)
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text: fmt.Sprintf("Бесплатный лимит инлайн-проверок исчерпан. Безлимитная подписка — %s/мес: отправьте /subscribe боту @chetoru_bot.\n\nВ самом боте проверка бесплатна: /check или .текст", SubscriptionPriceFormatted),
		}
		inlineConf := tgbotapi.InlineConfig{
			InlineQueryID: iq.ID,
			IsPersonal:    true,
			CacheTime:     0,
			Results:       []any{article},
		}
		_, _ = n.bot.Request(inlineConf)
		return nil
	}

	result, err := n.ai.SpellCheck(ctx, text)
	if err != nil {
		n.log.WithError(err).Error("ai.SpellCheck inline")
		return nil
	}

	var articles []any

	if result.NoErrors {
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+"_sp0", "✅ Ошибок не найдено", text)
		article.Description = text
		article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: text}
		articles = append(articles, article)
	} else if result.Corrected != "" {
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+"_sp0", "✏️ "+result.Corrected, result.Corrected)
		article.Description = "Нажмите, чтобы отправить исправленный текст"
		article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: result.Corrected}
		articles = append(articles, article)
	}

	// Only count a use when we actually produced a result for the user; an
	// empty/ambiguous AI response should not burn the free quota.
	if len(articles) > 0 {
		n.trackSpellcheckUsage(ctx, iq.From.ID)
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
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

	return nil
}

func (n *Net) HandleSpellcheckFeedback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	data := cq.Data
	parts := strings.SplitN(data, "_", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid spellcheck feedback format")
	}

	feedback := parts[1] // "like" or "dislike"
	msgText := cq.Message.Text

	var corrected string
	for _, line := range strings.Split(msgText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "✏️ ") {
			corrected = strings.TrimPrefix(line, "✏️ ")
			break
		}
	}

	if err := n.repo.StoreSpellcheckFeedback(ctx, cq.From.ID, msgText, corrected, feedback); err != nil {
		n.log.WithError(err).Error("repo.StoreSpellcheckFeedback")
	}

	var status string
	if feedback == "like" {
		status = "👍 Спасибо за отзыв!"
	} else {
		status = "👎 Спасибо, учтём!"
	}

	callback := tgbotapi.NewCallback(cq.ID, status)
	if _, err := n.bot.Request(callback); err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}

	// Remove buttons after feedback
	edited := tgbotapi.NewEditMessageReplyMarkup(
		cq.Message.Chat.ID,
		cq.Message.MessageID,
		tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}},
	)
	n.bot.Send(edited)

	return nil
}

func spellcheckFeedbackKeyboard(original, corrected string) tgbotapi.InlineKeyboardMarkup {
	hash := fmt.Sprintf("%d", len(original)+len(corrected))
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👍", "spell_like_"+hash),
			tgbotapi.NewInlineKeyboardButtonData("👎", "spell_dislike_"+hash),
		),
	)
}

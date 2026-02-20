package net

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleCheck(ctx context.Context, update *tgbotapi.Update) error {
	// Try command arguments first, then raw message text (for dot-prefix mode)
	text := strings.TrimSpace(update.Message.CommandArguments())
	if text == "" {
		text = strings.TrimSpace(update.Message.Text)
	}
	if text == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"Использование: /check <текст на чеченском>\n\nПример: /check дала безам бу хьо\n\nИли просто начни сообщение с точки:\n.дала безам бу хьо")
		_, err := n.bot.Send(msg)
		return err
	}

	if n.ai == nil {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⚠️ Проверка орфографии временно недоступна")
		_, err := n.bot.Send(msg)
		return err
	}

	n.bot.Send(tgbotapi.NewChatAction(update.Message.Chat.ID, tgbotapi.ChatTyping))

	result, err := n.ai.SpellCheck(ctx, text)
	if err != nil {
		n.log.WithError(err).Error("ai.SpellCheck")
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⚠️ Не удалось проверить текст, попробуйте позже")
		_, sendErr := n.bot.Send(msg)
		return sendErr
	}

	if result.NoErrors {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "✅ Ошибок не найдено")
		msg.ReplyToMessageID = update.Message.MessageID
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

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, responseText)
	msg.ReplyToMessageID = update.Message.MessageID

	if result.Corrected != "" {
		msg.ReplyMarkup = spellcheckFeedbackKeyboard(text, result.Corrected)
	}

	_, err = n.bot.Send(msg)
	return err
}

func (n *Net) HandleInlineSpellcheck(ctx context.Context, update *tgbotapi.Update) error {
	text := strings.TrimPrefix(update.InlineQuery.Query, ". ")

	if n.ai == nil {
		return nil
	}

	result, err := n.ai.SpellCheck(ctx, text)
	if err != nil {
		n.log.WithError(err).Error("ai.SpellCheck inline")
		return nil
	}

	var articles []interface{}

	if result.NoErrors {
		article := tgbotapi.NewInlineQueryResultArticle(update.InlineQuery.ID+"_sp0", "✅ Ошибок не найдено", text)
		article.Description = text
		article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: text}
		articles = append(articles, article)
	} else if result.Corrected != "" {
		article := tgbotapi.NewInlineQueryResultArticle(update.InlineQuery.ID+"_sp0", "✏️ "+result.Corrected, result.Corrected)
		article.Description = "Нажмите, чтобы отправить исправленный текст"
		article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: result.Corrected}
		articles = append(articles, article)
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

	return nil
}

func (n *Net) HandleSpellcheckFeedback(ctx context.Context, update *tgbotapi.Update) error {
	data := update.CallbackQuery.Data
	parts := strings.SplitN(data, "_", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid spellcheck feedback format")
	}

	feedback := parts[1] // "like" or "dislike"
	msgText := update.CallbackQuery.Message.Text

	var corrected string
	for _, line := range strings.Split(msgText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "✏️ ") {
			corrected = strings.TrimPrefix(line, "✏️ ")
			break
		}
	}

	if err := n.repo.StoreSpellcheckFeedback(ctx, update.CallbackQuery.From.ID, msgText, corrected, feedback); err != nil {
		n.log.WithError(err).Error("repo.StoreSpellcheckFeedback")
	}

	var status string
	if feedback == "like" {
		status = "👍 Спасибо за отзыв!"
	} else {
		status = "👎 Спасибо, учтём!"
	}

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, status)
	if _, err := n.bot.Request(callback); err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}

	// Remove buttons after feedback
	edited := tgbotapi.NewEditMessageReplyMarkup(
		update.CallbackQuery.Message.Chat.ID,
		update.CallbackQuery.Message.MessageID,
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

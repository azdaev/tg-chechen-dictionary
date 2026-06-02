package net

import (
	"chetoru/pkg/tools"
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandleRandom sends a random Chechen word from the dictionary so users can
// discover and learn new vocabulary, with a button to keep exploring.
func (n *Net) HandleRandom(ctx context.Context, chatID int64) error {
	// Prefer the API: it draws from the full 130K-entry dictionary. Fall back to
	// the local moderated table if the API is unavailable or returns nothing.
	word, err := n.business.RandomWordFromAPI(ctx)
	if err != nil {
		n.log.WithError(err).Warn("RandomWordFromAPI failed, falling back to local")
		word = nil
	}

	if word == nil {
		word, err = n.repo.RandomApprovedPair(ctx)
		if err != nil {
			return fmt.Errorf("repo.RandomApprovedPair: %w", err)
		}
	}

	if word == nil {
		_, err = n.bot.Send(tgbotapi.NewMessage(chatID, RandomEmptyText))
		return err
	}

	text := fmt.Sprintf(
		RandomWordFormat,
		tgbotapi.EscapeText(tgbotapi.ModeHTML, tools.Clean(word.Chechen)),
		tgbotapi.EscapeText(tgbotapi.ModeHTML, tools.Clean(word.Russian)),
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "html"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(RandomMoreButtonText, "random_more"),
		),
	)

	_, err = n.bot.Send(msg)
	return err
}

// HandleRandomCallback answers the "🎲 Ещё" button by sending another random word.
func (n *Net) HandleRandomCallback(ctx context.Context, update *tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := n.bot.Request(callback); err != nil {
		n.log.WithError(err).Warn("failed to ack random callback")
	}
	return n.HandleRandom(ctx, update.CallbackQuery.Message.Chat.ID)
}

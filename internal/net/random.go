package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// maxSummaryForms caps the inflected forms shown on compact cards like /random,
// where the word is the focus and grammar is a hint, not the content.
const maxSummaryForms = 6

// grammarSummaryLine renders a one-line italic grammar hint (part of speech and
// a few inflected forms) for compact cards. Returns "" when there is nothing to show.
func grammarSummaryLine(g *models.WordGrammar) string {
	if g == nil {
		return ""
	}
	var parts []string
	if g.POS != "" {
		parts = append(parts, g.POS)
	}
	if len(g.Forms) > 0 {
		forms := g.Forms
		if len(forms) > maxSummaryForms {
			forms = forms[:maxSummaryForms]
		}
		cleaned := make([]string, 0, len(forms))
		for _, f := range forms {
			cleaned = append(cleaned, tgbotapi.EscapeText(tgbotapi.ModeHTML, tools.Clean(f)))
		}
		parts = append(parts, "формы: "+strings.Join(cleaned, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "<i>" + strings.Join(parts, " · ") + "</i>"
}

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

	// Both enrichments hit live APIs on fresh words; fetch them concurrently so
	// the card costs one round trip, not two.
	var exampleLine, grammarLine string
	var wg sync.WaitGroup
	wg.Go(func() {
		if ex, ok := n.usageExample(word.Chechen); ok {
			exampleLine = fmt.Sprintf(WordOfDayExampleFormat, tgbotapi.EscapeText(tgbotapi.ModeHTML, ex))
		}
	})
	wg.Go(func() { grammarLine = grammarSummaryLine(n.business.GrammarFor(ctx, word.Chechen)) })
	wg.Wait()
	if exampleLine != "" {
		text += "\n\n" + exampleLine
	}
	if grammarLine != "" {
		text += "\n\n" + grammarLine
	}

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
func (n *Net) HandleRandomCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	callback := tgbotapi.NewCallback(cq.ID, "")
	if _, err := n.bot.Request(callback); err != nil {
		n.log.WithError(err).Warn("failed to ack random callback")
	}
	return n.HandleRandom(ctx, cq.Message.Chat.ID)
}

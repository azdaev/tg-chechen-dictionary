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

func (n *Net) HandleText(ctx context.Context, m *tgbotapi.Message) error {
	loaderMessage, err := n.bot.Send(tgbotapi.NewMessage(m.Chat.ID, "⌛️"))
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	defer func() {
		n.bot.Send(tgbotapi.NewDeleteMessage(m.Chat.ID, loaderMessage.MessageID))
	}()

	n.bot.Send(tgbotapi.NewChatAction(m.Chat.ID, tgbotapi.ChatTyping))

	if err := n.repo.StoreUser(ctx, int(m.From.ID), m.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	if err := n.repo.MarkUserUnblocked(ctx, m.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", m.From.ID).Warn("failed to unblock user")
	}

	if err := n.repo.StoreActivity(ctx, int(m.From.ID), models.ActivityTypeText); err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	result := n.business.TranslateFormatted(m.Text)
	if len(result.Pairs) == 0 {
		// Record the vocabulary gap so maintainers know what to add next.
		cleanWord := tools.NormalizeSearch(m.Text)
		if err := n.repo.RecordMissingWord(ctx, cleanWord, strings.TrimSpace(m.Text)); err != nil {
			n.log.WithError(err).WithField("word", cleanWord).Warn("failed to record missing word")
		}
		text := NoTranslationText
		if suggestions := n.business.SuggestTranslations(m.Text); len(suggestions) > 0 {
			text += "\n\n" + SuggestionsHeaderText + "\n\n" + tools.FormatPairs(suggestions)
		}
		msg := tgbotapi.NewMessage(m.Chat.ID, text)
		msg.ParseMode = "html"
		_, err = n.bot.Send(msg)
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
		messageText = tools.FormatPairs(firstTranslations)
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, messageText)
	msg.ParseMode = "html"

	if len(result.Pairs) > MaxTranslations {
		remainingCount := len(result.Pairs) - MaxTranslations
		// Only attach the "More" button when its callback data fits Telegram's
		// 64-byte limit; otherwise sending the whole message would fail. The
		// inline-mode hint below still tells users how to see all translations.
		if data, ok := moreCallbackData(m.Text, MaxTranslations); ok {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(
						fmt.Sprintf(MoreButtonText, remainingCount),
						data,
					),
				),
			)
		}
		msg.Text += "\n\n" + MoreTranslationsHelpText
	}

	if _, err = n.bot.Send(msg); err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	// Grammar card for the Chechen headword, sent as a follow-up so it never
	// delays the (usually cached) translation above. Runs detached because the
	// update loop processes messages synchronously and this makes a live API call.
	go n.sendGrammarCard(context.Background(), m.Chat.ID, m.Text)

	// Check if we should send a donation message
	shouldSend, err := n.repo.ShouldSendDonationMessage(ctx, int(m.From.ID))
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
		if err = n.repo.StoreDonationMessage(ctx, int(m.From.ID)); err != nil {
			return fmt.Errorf("failed to store donation message: %w", err)
		}
	}

	return nil
}

func (n *Net) HandleInline(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	translations := n.business.Translate(iq.Query)

	// Telegram allows at most 50 results per inline query; sending more makes
	// answerInlineQuery fail and the user sees nothing. Cap defensively — common
	// words (e.g. "дать") can have far more than 50 translation pairs.
	if len(translations) > InlineResultsLimit {
		translations = translations[:InlineResultsLimit]
	}

	articles := make([]any, 0, len(translations))
	for i := range translations {
		title := tools.Clean(translations[i].Original)
		if strings.TrimSpace(title) == "" {
			// Telegram rejects the entire answer if any article title is empty,
			// so one malformed entry would blank out the whole inline response.
			continue
		}
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+strconv.Itoa(i), title, "")
		article.Description = tools.Clean(translations[i].Translate)
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text: fmt.Sprintf("<b>%s</b> - %s",
				tgbotapi.EscapeText(tgbotapi.ModeHTML, tools.Clean(translations[i].Original)),
				tgbotapi.EscapeText(tgbotapi.ModeHTML, tools.Clean(translations[i].Translate))),
			ParseMode: "html",
		}
		articles = append(articles, article)
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

	if err := n.repo.StoreUser(ctx, int(iq.From.ID), iq.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}
	if err := n.repo.MarkUserUnblocked(ctx, iq.From.ID); err != nil {
		n.log.WithError(err).WithField("user_id", iq.From.ID).Warn("failed to unblock user")
	}
	if err := n.repo.StoreActivity(ctx, int(iq.From.ID), models.ActivityTypeInline); err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	return nil
}

func (n *Net) HandleMoreTranslations(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	word, offset, ok := parseMoreCallback(cq.Data)
	if !ok {
		return fmt.Errorf("invalid more callback data: %q", cq.Data)
	}

	// Always acknowledge the callback so the client's loading spinner clears.
	defer func() {
		if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
			n.log.WithError(err).Warn("failed to ack more callback")
		}
	}()

	translations := n.business.Translate(word)
	if len(translations) == 0 {
		_, err := n.bot.Send(tgbotapi.NewMessage(cq.Message.Chat.ID, NoTranslationText))
		return err
	}

	// Clamp the offset: the result set may have shrunk since the button was
	// created (cache expiry, API change), which would otherwise panic on slice.
	if offset < 0 {
		offset = 0
	}
	if offset >= len(translations) {
		return nil // nothing more to show; callback already acked
	}

	end := min(offset+MaxTranslations, len(translations))
	nextTranslations := translations[offset:end]

	msg := tgbotapi.NewMessage(cq.Message.Chat.ID, tools.FormatPairs(nextTranslations))
	msg.ParseMode = "html"

	if end < len(translations) {
		if data, ok := moreCallbackData(word, end); ok {
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(
						fmt.Sprintf(MoreButtonText, len(translations)-end),
						data,
					),
				),
			)
		}
	}

	if _, err := n.bot.Send(msg); err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}
	return nil
}

// moreCallbackData builds the "More" button payload. Telegram caps callback_data
// at 64 bytes, so it returns ok=false when the word is too long to encode; the
// caller then omits the button rather than letting the whole message send fail.
func moreCallbackData(word string, offset int) (string, bool) {
	data := fmt.Sprintf("more_%s_%d", word, offset)
	return data, len(data) <= 64
}

// parseMoreCallback parses "more_<word>_<offset>". The offset is read from the
// last underscore so words containing underscores are handled correctly.
func parseMoreCallback(data string) (word string, offset int, ok bool) {
	rest, found := strings.CutPrefix(data, "more_")
	if !found {
		return "", 0, false
	}
	idx := strings.LastIndex(rest, "_")
	if idx <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(rest[idx+1:])
	if err != nil || n < 0 {
		return "", 0, false
	}
	return rest[:idx], n, true
}

// maxGrammarForms caps how many inflected forms the grammar card lists, so a
// word with a long paradigm doesn't produce an overwhelming message.
const maxGrammarForms = 12

// sendGrammarCard looks up grammar for the Chechen headword behind a query and,
// if any is available, sends a compact follow-up card. It is a no-op when the
// word has no analyzed grammar, so most TEXT/phrase lookups send nothing.
func (n *Net) sendGrammarCard(ctx context.Context, chatID int64, word string) {
	g := n.business.GrammarFor(ctx, word)
	if g == nil {
		return
	}
	text := formatGrammarCard(g)
	if text == "" {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "html"
	msg.DisableNotification = true
	if _, err := n.bot.Send(msg); err != nil {
		n.log.WithError(err).Warn("failed to send grammar card")
	}
}

// formatGrammarCard renders a WordGrammar as a small Telegram-HTML card. Only
// facts safe to show without the dosham integer-code legend are included: the
// part of speech (when confidently known) and the inflected forms.
func formatGrammarCard(g *models.WordGrammar) string {
	if g == nil || g.Headword == "" {
		return ""
	}
	header := "📖 <b>" + tools.Clean(g.Headword) + "</b>"
	if g.POS != "" {
		header += " · " + g.POS
	}

	lines := []string{header}
	if len(g.Forms) > 0 {
		forms := g.Forms
		more := 0
		if len(forms) > maxGrammarForms {
			more = len(forms) - maxGrammarForms
			forms = forms[:maxGrammarForms]
		}
		cleaned := make([]string, 0, len(forms))
		for _, f := range forms {
			cleaned = append(cleaned, tools.Clean(f))
		}
		line := "Формы: " + strings.Join(cleaned, ", ")
		if more > 0 {
			line += fmt.Sprintf(" … (+%d)", more)
		}
		lines = append(lines, line)
	}

	if len(g.Idioms) > 0 {
		lines = append(lines, "\n💬 <b>Выражения:</b>")
		for _, idiom := range g.Idioms {
			lines = append(lines, fmt.Sprintf("• %s — %s", tools.Clean(idiom.Chechen), tools.Clean(idiom.Russian)))
		}
	}

	if len(lines) == 1 && g.POS == "" {
		return "" // only a bare headword — nothing useful to show
	}
	return strings.Join(lines, "\n")
}

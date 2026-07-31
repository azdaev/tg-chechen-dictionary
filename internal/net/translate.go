package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleText(ctx context.Context, m *tgbotapi.Message) error {
	// A typing indicator instead of the old ⌛️ loader message: the loader cost
	// two blocking Telegram round trips (send + delete) on every lookup before
	// any work started. Fire-and-forget so even this one doesn't delay the answer.
	go n.bot.Send(tgbotapi.NewChatAction(m.Chat.ID, tgbotapi.ChatTyping))

	// Bookkeeping runs after the reply has been sent — storage writes should
	// never sit between the user and the translation.
	defer n.recordActivity(ctx, m.From.ID, m.From.UserName, models.ActivityTypeText)

	translations := n.business.Translate(m.Text)
	if len(translations) == 0 {
		// Record the vocabulary gap so maintainers know what to add next —
		// detached, so the write never delays the reply the user is waiting on.
		n.bg.Go(func() {
			cleanWord := tools.NormalizeSearch(m.Text)
			if !isRecordableMissingWord(cleanWord) {
				return
			}
			if err := n.repo.RecordMissingWord(ctx, cleanWord, strings.TrimSpace(m.Text)); err != nil {
				n.log.WithError(err).WithField("word", cleanWord).Warn("failed to record missing word")
			}
		})
		text := NoTranslationText
		if suggestions := n.business.SuggestTranslations(m.Text); len(suggestions) > 0 {
			text += "\n\n" + SuggestionsHeaderText + "\n\n" + tools.FormatPairs(suggestions)
		}
		msg := tgbotapi.NewMessage(m.Chat.ID, text)
		msg.ParseMode = "html"
		_, err := n.bot.Send(msg)
		return err
	}

	// A successful lookup proves the word is covered now — clear it from the
	// missing-words report so the gap list reflects only live gaps. A no-op
	// delete for the common case (word was never missing) is a btree probe.
	n.bg.Go(func() {
		cleanWord := tools.NormalizeSearch(m.Text)
		if err := n.repo.ResolveMissingWord(ctx, cleanWord); err != nil {
			n.log.WithError(err).WithField("word", cleanWord).Warn("failed to resolve missing word")
		}
	})

	firstTranslations := translations
	if len(translations) > MaxTranslations {
		firstTranslations = translations[:MaxTranslations]
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, clampMessage(tools.FormatPairs(firstTranslations)))
	msg.ParseMode = "html"

	if len(translations) > MaxTranslations {
		remainingCount := len(translations) - MaxTranslations
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

	if _, err := n.bot.Send(msg); err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	// Grammar card for the Chechen headword, sent as a follow-up so it never
	// delays the (usually cached) translation above. Runs detached because the
	// update loop processes messages synchronously and this makes a live API call.
	n.bg.Go(func() { n.sendGrammarCard(context.Background(), m.Chat.ID, m.Text) })

	// Donation nudge runs detached: it is a DB check plus an extra Telegram
	// message per lookup, and was the last synchronous roundtrip in this tail.
	n.bg.Go(func() { n.maybeSendDonation(context.Background(), m.Chat.ID, int(m.From.ID)) })

	if m.Chat.Type == "private" {
		n.bg.Go(func() { n.maybeSuggestWordOfDay(context.Background(), m.Chat.ID, m.From.ID) })
	}

	return nil
}

// maybeSendDonation sends the periodic donation ask if the user is due one.
// Failures are logged, not returned — the translation is already delivered.
func (n *Net) maybeSendDonation(ctx context.Context, chatID int64, userID int) {
	shouldSend, err := n.repo.ShouldSendDonationMessage(ctx, userID)
	if err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to check donation message status")
		return
	}
	if !shouldSend {
		return
	}

	donationMsg := tgbotapi.NewMessage(chatID, DonationMessageFormat)
	donationMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🚀 Поддержать нас", os.Getenv("DONATION_LINK")),
		),
	)
	if _, err := n.bot.Send(donationMsg); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to send donation message")
		return
	}
	if err := n.repo.StoreDonationMessage(ctx, userID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to store donation message")
	}
}

// inlineDescriptionRunes caps the inline picker's subtitle. Telegram truncates
// longer ones itself, but a gloss cut mid-word by the client reads worse than
// one cut at a boundary here.
const inlineDescriptionRunes = 100

// inlineDescription renders the one-line subtitle under an inline result. It
// takes the raw gloss rather than the rendered card, so it is plain text by
// construction.
func inlineDescription(gloss string) string {
	desc := strings.Join(strings.Fields(tools.Clean(gloss)), " ")
	runes := []rune(desc)
	if len(runes) <= inlineDescriptionRunes {
		return desc
	}
	cut := string(runes[:inlineDescriptionRunes])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}

// clampMessage keeps text under Telegram's message-length cap; an oversized
// message is rejected outright, so the user would get nothing at all. The cut
// lands on a line boundary so HTML tags inside a line aren't split open.
func clampMessage(text string) string {
	// Telegram's limit is 4096; leave margin for the help-text suffixes some
	// callers append after clamping.
	const limit = 3800
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	cut := string(runes[:limit])
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return cut + "\n…"
}

// isRecordableMissingWord filters dead-end searches before they reach the
// missing-words report: URLs, Latin-only strings, digits and whole sentences
// are not dictionary candidates and would bury the real gaps.
func isRecordableMissingWord(cleanWord string) bool {
	runes := []rune(cleanWord)
	if len(runes) < 2 || len(runes) > 40 {
		return false
	}
	if strings.Count(cleanWord, " ") > 2 {
		return false
	}
	hasCyrillic := false
	for _, r := range runes {
		switch {
		case r >= 'а' && r <= 'я' || r == 'ё' || r == 'ӏ':
			hasCyrillic = true
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			return false
		case unicode.IsDigit(r) && r != '1':
			// "1" stays: it is how Russian keyboards type the palochka
			// ("къинт1ера" for "къинтӏера").
			return false
		}
	}
	return hasCyrillic
}

// recordActivity persists per-user bookkeeping for a lookup. Failures are
// logged, not returned — the reply has already been sent.
func (n *Net) recordActivity(ctx context.Context, userID int64, username string, activityType models.ActivityType) {
	if err := n.repo.RecordUserActivity(ctx, userID, username, activityType); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to record activity")
	}
}

func (n *Net) HandleInline(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	// An empty query is the "@bot " moment in someone's chat — serve a few
	// discovery words instead of a blank screen.
	if strings.TrimSpace(iq.Query) == "" {
		return n.answerInlineDiscovery(ctx, iq)
	}

	translations := n.business.Translate(iq.Query)

	// A dead-end inline query used to show nothing at all; rescue it the same
	// way the text path does — with lemma suggestions for the typed prefix.
	suggested := false
	if len(translations) == 0 {
		translations = n.business.SuggestTranslations(iq.Query)
		suggested = len(translations) > 0
	}

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
		if suggested {
			title = "🔍 " + title
		}
		// The sent message gets the same card the text path produces, instead
		// of dumping the raw gloss ("м 1) цӏа; деревянный ~- …").
		formatted := clampMessage(tools.FormatPairs(translations[i : i+1]))
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+strconv.Itoa(i), title, "")
		// The description comes from the data, not from the rendered card: the
		// picker shows plain text, so a card carrying <b> would leak the literal
		// tags, and slicing a headword prefix off the front breaks the moment
		// the card's opening line changes.
		article.Description = inlineDescription(translations[i].Translate)
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text:      formatted,
			ParseMode: "html",
		}
		articles = append(articles, article)
	}

	// Dictionary results are identical for everyone and effectively static, so
	// let Telegram cache them on its edge: repeated queries (every keystroke
	// counts as one) are then answered without reaching the bot at all. The
	// spellcheck inline path stays personal — it has per-user quotas.
	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		IsPersonal:    false,
		CacheTime:     InlineCacheTimeSeconds,
		Results:       articles,
	}

	resp, err := n.bot.Request(inlineConf)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}
	if !resp.Ok {
		return fmt.Errorf("bot.Request: %s", resp.Description)
	}

	n.recordActivity(ctx, iq.From.ID, iq.From.UserName, models.ActivityTypeInline)
	return nil
}

// answerInlineDiscovery responds to an empty inline query with a few random
// dictionary words. Pool-backed, so the common case costs no API call.
func (n *Net) answerInlineDiscovery(ctx context.Context, iq *tgbotapi.InlineQuery) error {
	articles := make([]any, 0, InlineDiscoveryCount)
	for i := range InlineDiscoveryCount {
		w, err := n.business.RandomWordFromAPI(ctx)
		if err != nil || w == nil {
			break
		}
		text := fmt.Sprintf(
			RandomWordFormat,
			tgbotapi.EscapeText(tgbotapi.ModeHTML, w.Chechen),
			tgbotapi.EscapeText(tgbotapi.ModeHTML, w.Russian),
		)
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+strconv.Itoa(i), "🎲 "+w.Chechen, "")
		article.Description = w.Russian
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text:      text,
			ParseMode: "html",
		}
		articles = append(articles, article)
	}
	if len(articles) == 0 {
		return nil // pool empty and API down; leave the placeholder screen
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		IsPersonal:    false,
		CacheTime:     InlineDiscoveryCacheSec,
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

	msg := tgbotapi.NewMessage(cq.Message.Chat.ID, clampMessage(tools.FormatPairs(nextTranslations)))
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
	// 🔤, not 📖: the book belongs to the Word of the Day, and a subscriber who
	// gets both opens the grammar card reading it as today's word.
	header := "🔤 <b>" + tools.Clean(g.Headword) + "</b>"
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
			lines = append(lines, "• "+tools.FormatExample(tools.Clean(idiom.Chechen), tools.Clean(idiom.Russian)))
		}
	}

	if len(lines) == 1 && g.POS == "" {
		return "" // only a bare headword — nothing useful to show
	}
	return strings.Join(lines, "\n")
}

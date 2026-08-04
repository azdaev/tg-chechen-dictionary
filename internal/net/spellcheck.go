package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// spellcheck runs an AI check through the Redis cache. Verdicts are
// deterministic for a given text, and identical texts recur (shared phrases,
// re-checks after edits elsewhere), so a hit skips the OpenRouter call.
func (n *Net) spellcheck(ctx context.Context, text string) (*ai.SpellCheckResult, error) {
	cached, err := n.cache.GetSpellcheck(ctx, text)
	if err == nil {
		return cached, nil
	}
	if !errors.Is(err, cache.ErrMiss) {
		n.log.WithError(err).Warn("spellcheck cache read failed")
	}

	result, err := n.ai.SpellCheck(ctx, text)
	if err != nil {
		return nil, err
	}
	if err := n.cache.SetSpellcheck(ctx, text, result); err != nil {
		n.log.WithError(err).Warn("spellcheck cache write failed")
	}
	return result, nil
}

// checkTarget is the text /check should run on: its arguments, or the whole
// message in dot-prefix mode («.дала безам бу»). A bare /check has neither, and
// used to fall through to the message itself — spending one of the five free
// monthly checks to be told that «/check» is misspelled, and never reaching the
// usage text sitting right below.
func checkTarget(m *tgbotapi.Message) string {
	if args := strings.TrimSpace(m.CommandArguments()); args != "" {
		return args
	}
	if m.IsCommand() {
		return ""
	}
	return strings.TrimSpace(m.Text)
}

func (n *Net) HandleCheck(ctx context.Context, m *tgbotapi.Message) error {
	text := checkTarget(m)
	if text == "" {
		msg := tgbotapi.NewMessage(m.Chat.ID,
			"Использование: /check <текст на чеченском>\n\nПример: /check дала безам бу хьо\n\nИли просто начни сообщение с точки:\n.дала безам бу хьо")
		_, err := n.send(msg)
		return err
	}

	return n.runSpellcheck(ctx, m.Chat.ID, m.MessageID, text)
}

// runSpellcheck checks text and replies with the verdict. Shared by /check, the
// dot-prefix shortcut, and the button offered after a failed lookup — a typo is
// the most common reason a word is not found, and the checker was already here.
func (n *Net) runSpellcheck(ctx context.Context, chatID int64, replyTo int, text string) error {
	if n.ai == nil {
		msg := tgbotapi.NewMessage(chatID, "⚠️ Проверка орфографии временно недоступна")
		_, err := n.send(msg)
		return err
	}

	n.send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))

	result, err := n.spellcheck(ctx, text)
	if err != nil {
		n.log.WithError(err).Error("ai.SpellCheck")
		msg := tgbotapi.NewMessage(chatID, "⚠️ Не удалось проверить текст, попробуйте позже")
		_, sendErr := n.send(msg)
		return sendErr
	}

	if result.NoErrors {
		msg := tgbotapi.NewMessage(chatID, "✅ Ошибок не найдено")
		msg.ReplyToMessageID = replyTo
		msg.AllowSendingWithoutReply = true
		_, err = n.send(msg)
		return err
	}

	// Format response
	var responseText string
	if result.Corrected != "" {
		responseText = "✏️ " + markCorrections(text, result.Corrected)
	}
	if idx := strings.Index(result.Explanation, "CHANGES:"); idx != -1 {
		changes := strings.TrimSpace(result.Explanation[idx+len("CHANGES:"):])
		if changes != "" {
			responseText += "\n\n📝 Изменения:\n" + tgbotapi.EscapeText(tgbotapi.ModeHTML, changes)
		}
	}
	if responseText == "" {
		responseText = tgbotapi.EscapeText(tgbotapi.ModeHTML, result.Explanation)
	}

	msg := tgbotapi.NewMessage(chatID, responseText)
	// The one card where bold carries information rather than decoration: which
	// words the checker actually touched. A parse failure falls back to plain
	// text on its own, so the markup can only add.
	msg.ParseMode = "html"
	msg.ReplyToMessageID = replyTo
	msg.AllowSendingWithoutReply = true

	if result.Corrected != "" {
		msg.ReplyMarkup = spellcheckFeedbackKeyboard(text, result.Corrected)
	}

	_, err = n.send(msg)
	return err
}

// checkCallbackData builds the payload for the "check the spelling" button
// offered after a failed lookup. Telegram caps callback_data at 64 bytes and
// Cyrillic costs two per letter, so it reports ok=false for a long query and
// the caller omits the button rather than letting the whole send fail.
func checkCallbackData(text string) (string, bool) {
	data := "check_" + strings.TrimSpace(text)
	return data, len(data) <= 64
}

// HandleSpellcheckRequest answers the button from a failed lookup by running
// the checker on the word the user typed.
func (n *Net) HandleSpellcheckRequest(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
		n.log.WithError(err).Warn("failed to ack spellcheck callback")
	}
	text, found := strings.CutPrefix(cq.Data, "check_")
	if !found || text == "" {
		return fmt.Errorf("invalid spellcheck callback data: %q", cq.Data)
	}

	// Metered, unlike /check. The button sits under every failed lookup, so
	// without a quota one typo-prone session is an unbounded number of AI calls
	// nobody chose to make — /check at least has to be typed.
	allowed, err := n.canUseSpellcheck(ctx, cq.From.ID)
	switch spellcheckAccessFor(allowed, err) {
	case spellcheckUnreadable:
		n.log.WithError(err).Warn("canUseSpellcheck button")
		_, sendErr := n.send(tgbotapi.NewMessage(cq.Message.Chat.ID, SpellcheckUnavailableText))
		return sendErr
	case spellcheckPaywalled:
		msg := tgbotapi.NewMessage(cq.Message.Chat.ID, fmt.Sprintf(
			"Бесплатный лимит проверок исчерпан (%d/мес). Безлимитная подписка — %s/мес: /subscribe",
			FreeSpellcheckLimit, SubscriptionPriceFormatted))
		_, err := n.send(msg)
		return err
	}
	if err := n.runSpellcheck(ctx, cq.Message.Chat.ID, cq.Message.MessageID, text); err != nil {
		return err
	}
	n.trackSpellcheckUsage(ctx, cq.From.ID)
	return nil
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

	allowed, err := n.canUseSpellcheck(ctx, iq.From.ID)
	switch spellcheckAccessFor(allowed, err) {
	case spellcheckUnreadable:
		n.log.WithError(err).Error("canUseSpellcheck inline")
		article := tgbotapi.NewInlineQueryResultArticle(iq.ID+"_err", "⚠️ Проверка недоступна", "")
		article.Description = "Попробуйте через минуту"
		article.InputMessageContent = tgbotapi.InputTextMessageContent{Text: SpellcheckUnavailableText}
		return n.answerInline(tgbotapi.InlineConfig{
			InlineQueryID: iq.ID,
			IsPersonal:    true,
			CacheTime:     0,
			Results:       []any{article},
		})
	case spellcheckPaywalled:
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
		_ = n.answerInline(inlineConf)
		return nil
	}

	result, err := n.spellcheck(ctx, text)
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

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		IsPersonal:    true,
		CacheTime:     0,
		Results:       articles,
	}

	if err := n.answerInline(inlineConf); err != nil {
		return fmt.Errorf("answerInline: %w", err)
	}

	// Counted only once the answer actually reached Telegram, and only when
	// there was one to reach it. Charged before the send, a timeout or a
	// rejected result cost the user one of five monthly checks for a screen
	// that stayed empty.
	if len(articles) > 0 {
		n.trackSpellcheckUsage(ctx, iq.From.ID)
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
	for line := range strings.SplitSeq(msgText, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "✏️ "); ok {
			corrected = rest
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
	n.send(edited)

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

// markCorrections bolds the words the checker changed. A correction the reader
// has to find by comparing two spellings letter by letter is barely a
// correction — and the words most often fixed here differ from what was typed
// by one palochka.
//
// Compared as typed, lowercased and nothing else: folding the palochka away
// would hide precisely the fix it is here to show.
func markCorrections(original, corrected string) string {
	typed := map[string]bool{}
	for _, w := range words(original) {
		typed[strings.ToLower(w)] = true
	}
	var b strings.Builder
	for _, tok := range tokens(corrected) {
		esc := tgbotapi.EscapeText(tgbotapi.ModeHTML, tok)
		if isWordToken(tok) && !typed[strings.ToLower(tok)] {
			esc = "<b>" + esc + "</b>"
		}
		b.WriteString(esc)
	}
	return b.String()
}

func words(text string) []string {
	var out []string
	for _, tok := range tokens(text) {
		if isWordToken(tok) {
			out = append(out, tok)
		}
	}
	return out
}

// tokens splits text into alternating runs of word characters and everything
// else, so rejoining them reproduces the input exactly — spacing, punctuation
// and line breaks included.
func tokens(text string) []string {
	var out []string
	start := 0
	var inWord bool
	for i, r := range text {
		w := isWordRune(r)
		if i == 0 {
			inWord = w
			continue
		}
		if w != inWord {
			out = append(out, text[start:i])
			start, inWord = i, w
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

func isWordToken(tok string) bool {
	for _, r := range tok {
		return isWordRune(r)
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\''
}

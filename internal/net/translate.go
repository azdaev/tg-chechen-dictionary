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
	go n.send(tgbotapi.NewChatAction(m.Chat.ID, tgbotapi.ChatTyping))

	// Bookkeeping runs after the reply has been sent — storage writes should
	// never sit between the user and the translation.
	defer n.recordActivity(ctx, m.From.ID, m.From.UserName, models.ActivityTypeText)

	translations, resolved, err := n.business.TranslateResolved(m.Text)
	if err != nil {
		// Not a miss: nothing gets recorded as a vocabulary gap, and
		// SuggestTranslations is skipped — it would fan one failed lookup out
		// into four more against the provider that just failed.
		n.log.WithError(err).WithField("word", m.Text).Warn("translation lookup failed")
		_, sendErr := n.send(tgbotapi.NewMessage(m.Chat.ID, DictionaryUnavailableText))
		return sendErr
	}
	// Pairs coming back is not the same as an answer coming back: dosham's
	// substring search matches «стрим» inside «гольфстрим» and «лоьма» inside
	// «Лоьма-кӏорца». The card is what decides, because it is the thing that
	// knows whether any entry actually means the query.
	// Rendered against the headword that answered, not the typed word: the
	// word-forms layer resolves «лоьман» to «лом», and a card keyed on the form
	// matches none of the lemma's pairs and comes back empty.
	renderKey := m.Text
	if resolved != "" {
		renderKey = resolved
	}
	// One parse either way: the two renderers share every judgement about what a
	// card says and differ only in the tags they say it with, but each runs the
	// article parser, and running both would run it twice per lookup.
	rendered := tools.Render(renderKey, translations)
	if richMessages {
		rendered = tools.RenderRich(renderKey, translations)
	}
	if rendered.Body == "" {
		return n.sendMiss(ctx, m, rendered.Neighbours)
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

	// One card holds the whole answer now, so there is no second page to offer:
	// what «Ещё» used to paginate was the noise dosham's substring search
	// returns, which the card drops instead of deferring.
	// A card that shows the word in use but never says what it means is half an
	// answer: «собаку» comes back as six sentences that contain it and no entry
	// of its own, so the user reads «жӏаьла караӏамо — выдрессировать собаку»
	// and still does not learn that a dog is жӏаьла. The lemma is one prefix
	// lookup away, and the miss path already knows how to find it. Offered, not
	// asserted — a guess at the lemma is not the same thing as an entry.
	var suggestions []models.TranslationPairs
	if !rendered.Glossed {
		suggestions = n.business.SuggestTranslations(m.Text)
	}
	hintInline := len(translations) > MaxTranslations && n.shouldHintInline(ctx, m.From.ID)

	// Which pieces the message is made of, and in what order, is decided once;
	// only the tags between them depend on how it goes out. The plain assembly
	// has to stay reachable because a rich send can fail.
	assemble := func(rich bool, body string, neighbours []string) string {
		b := cardBuilder{rich: rich}
		if tools.NormalizeSearch(renderKey) != tools.NormalizeSearch(m.Text) {
			b.quote(fmt.Sprintf(ResolvedQueryFormat, tgbotapi.EscapeText(tgbotapi.ModeHTML, m.Text)))
		}
		b.body(body)
		b.line(tools.FormatNeighbours(neighbours))
		if len(suggestions) > 0 {
			b.line(SuggestionsHeaderText)
			b.line(tools.FormatSuggestions(suggestions))
		}
		if hintInline {
			b.line(MoreTranslationsHelpText)
		}
		b.credit()
		return b.String()
	}

	built := assemble(richMessages, rendered.Body, rendered.Neighbours)
	// With the feature off the card is already plain, and the fallback must not
	// parse the article a second time to rediscover that.
	plainCard := func() string { return built }
	if richMessages {
		plainCard = func() string {
			plain := tools.Render(renderKey, translations)
			return assemble(false, plain.Body, plain.Neighbours)
		}
	}
	sent, card, rich, err := n.sendMaybeRich(m.Chat.ID, built, plainCard, nil)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	if hintInline {
		// Marked only after it actually reached someone: a failed send means
		// the lesson was never taught.
		n.bg.Go(func() {
			if err := n.repo.MarkInlineHinted(ctx, m.From.ID); err != nil {
				n.log.WithError(err).WithField("user_id", m.From.ID).Warn("failed to mark inline hint")
			}
		})
	}

	// Grammar for the headword the user is actually looking at — not for what
	// they typed, which may be an inflected form that matched a different entry.
	// Detached so it never delays the translation above, then edited into that
	// same message: the update loop is synchronous and this makes a live API
	// call, but the answer stays one message.
	headword := translations[0].Original
	if rendered.Chechen != "" {
		headword = rendered.Chechen
	}
	cardText := card
	base := tools.DerivedFrom(renderKey, translations)
	n.bg.Go(func() {
		n.sendGrammarCard(context.Background(), m.Chat.ID, sent.MessageID, cardText, headword, base, rich)
	})

	// Donation nudge runs detached: it is a DB check plus an extra Telegram
	// message per lookup, and was the last synchronous roundtrip in this tail.
	n.bg.Go(func() { n.maybeSendDonation(context.Background(), m.Chat.ID, int(m.From.ID)) })

	if m.Chat.Type == "private" {
		n.bg.Go(func() { n.maybeSuggestWordOfDay(context.Background(), m.Chat.ID, m.From.ID) })
	}

	return nil
}

// sendMiss answers a lookup that produced no meaning. Neighbours may still be
// worth showing, but only as a hint inside the miss — served on their own they
// read as an answer, and the gap never reached the missing-words report.
func (n *Net) sendMiss(ctx context.Context, m *tgbotapi.Message, neighbours []string) error {
	cleanWord := tools.NormalizeSearch(m.Text)
	// Whether the gap is worth recording also decides what we tell the user:
	// promising that a URL or a whole sentence went on the list would be a lie,
	// and offering to spellcheck one is no use either.
	recordable := isRecordableMissingWord(cleanWord)
	if recordable {
		// Detached, so the write never delays the reply the user is waiting on.
		n.bg.Go(func() {
			if err := n.repo.RecordMissingWord(ctx, cleanWord, strings.TrimSpace(m.Text)); err != nil {
				n.log.WithError(err).WithField("word", cleanWord).Warn("failed to record missing word")
			}
		})
	}

	// The same pieces in the same order for both dialects; only what separates
	// them differs. The hint is a quotation rather than one more paragraph — it
	// is the bot talking about the keyboard, not about the word.
	miss := func(rich bool) string {
		b := cardBuilder{rich: rich}
		b.line(NoTranslationText)
		// Straight after the bad news, not below the suggestions, where it read
		// as a remark about whichever near-miss happened to be last.
		if recordable {
			b.line(MissingWordRecordedText)
		}
		// Only for someone who did not type a palochka. The hint teaches a
		// keyboard trick, and a query that already carries «ӏ» — typed as a
		// digit or not, since the key is normalized by then — is from someone
		// who knows it. Four blocks of consolation on a miss is enough without a
		// lesson they have already learned.
		if tools.LooksChechen(cleanWord) && !strings.ContainsRune(cleanWord, 'ӏ') {
			b.quote(PalochkaHintText)
		}
		b.line(tools.FormatNeighbours(neighbours))
		if suggestions := n.business.SuggestTranslations(m.Text); len(suggestions) > 0 {
			b.line(SuggestionsHeaderText)
			b.line(tools.FormatSuggestions(suggestions))
		}
		return b.String()
	}

	// A miss used to be a dead end. It now says what happened to the word and
	// offers the one thing that most often explains it — a typo, which the
	// checker already knows how to find.
	var markup any
	if recordable && n.ai != nil {
		if data, ok := checkCallbackData(m.Text); ok {
			markup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(CheckSpellingButtonText, data),
				),
			)
		}
	}

	// Clamped like every other card: three long glosses clear 4096 characters,
	// and Telegram answers an oversized message by sending nothing — turning a
	// near miss into a blank screen.
	_, _, _, err := n.sendMaybeRich(m.Chat.ID, miss(richMessages),
		func() string { return miss(false) }, markup)
	return err
}

// shouldHintInline reports whether this user still needs the inline-mode
// lesson. A failed check stays silent: the hint is optional, and repeating it
// is the thing being fixed.
func (n *Net) shouldHintInline(ctx context.Context, userID int64) bool {
	hinted, err := n.repo.WasInlineHinted(ctx, userID)
	if err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("inline hint: check failed")
		return false
	}
	return !hinted
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
	if _, err := n.send(donationMsg); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to send donation message")
		return
	}
	if err := n.repo.StoreDonationMessage(ctx, userID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("failed to store donation message")
	}
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
	} else if i := strings.LastIndex(cut, "<"); i > strings.LastIndex(cut, ">") {
		// No line boundary to fall back on, so the cut can land inside a tag.
		// Telegram rejects the whole message over one broken tag and the retry
		// resends it unformatted, losing every mark the card makes meaning with.
		cut = cut[:i]
	}
	// The surviving text may still open a tag it never closes, for the same
	// reason. Nothing here nests, so the order the closers go in does not matter.
	for _, tag := range []string{"b", "i"} {
		if strings.Count(cut, "<"+tag+">") > strings.Count(cut, "</"+tag+">") {
			cut += "</" + tag + ">"
		}
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

	translations, err := n.business.Translate(word)
	if err != nil {
		n.log.WithError(err).WithField("word", word).Warn("more-translations lookup failed")
		_, sendErr := n.send(tgbotapi.NewMessage(cq.Message.Chat.ID, DictionaryUnavailableText))
		return sendErr
	}
	if len(translations) == 0 {
		_, err := n.send(tgbotapi.NewMessage(cq.Message.Chat.ID, NoTranslationText))
		return err
	}

	// New answers carry no «Ещё» button — the card is the whole answer. This
	// still fires for buttons sitting in older chats, and re-sends that card
	// rather than a page of the noise the button used to leaf through.
	_ = offset
	card := tools.FormatCard(word, translations)
	if card == "" {
		_, err := n.send(tgbotapi.NewMessage(cq.Message.Chat.ID, NoTranslationText))
		return err
	}
	msg := tgbotapi.NewMessage(cq.Message.Chat.ID, clampMessage(card))
	msg.ParseMode = "html"

	if _, err := n.send(msg); err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}
	return nil
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

// baseWordLine states what the word an entry is built from means. The
// dictionary defines a causative or a potential only by naming its base —
// «яхчийта — понуд. от яхча» — so without this the card holds two Chechen
// words and no Russian. One extra lookup, on the 8% of Chechen glosses that
// are nothing but such a pointer; a failure just leaves the card as it was.
func (n *Net) baseWordLine(base string) string {
	pairs, resolved, err := n.business.TranslateResolved(base)
	if err != nil || len(pairs) == 0 {
		return ""
	}
	// Whatever the bot answers for the base word is what the reader would get
	// by looking it up themselves, redirects included: «яхча» is filed under
	// «дахча», and the gloss found there is the one that belongs on this line.
	gloss := tools.FirstGloss(base, pairs)
	if gloss == "" && resolved != "" {
		gloss = tools.FirstGloss(resolved, pairs)
	}
	if gloss == "" {
		return ""
	}
	return fmt.Sprintf(DerivedFromFormat, tools.Clean(base), gloss)
}

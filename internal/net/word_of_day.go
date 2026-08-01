package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandleWordOfDay shows the Word of the Day subscription status with a button
// to opt in or out. In a group the subscription belongs to the chat itself —
// the word arrives where the community talks, not in members' private chats.
func (n *Net) HandleWordOfDay(ctx context.Context, m *tgbotapi.Message) error {
	var subscribed bool
	var err error
	if isGroup(m.Chat) {
		subscribed, err = n.repo.IsChatWordOfDaySubscribed(ctx, m.Chat.ID)
		if err != nil {
			return fmt.Errorf("repo.IsChatWordOfDaySubscribed: %w", err)
		}
	} else {
		if err := n.repo.StoreUser(ctx, int(m.From.ID), m.From.UserName); err != nil {
			return fmt.Errorf("repo.StoreUser: %w", err)
		}
		subscribed, err = n.repo.IsWordOfDaySubscribed(ctx, m.From.ID)
		if err != nil {
			return fmt.Errorf("repo.IsWordOfDaySubscribed: %w", err)
		}
	}

	out := tgbotapi.NewMessage(m.Chat.ID, wotdStatusText(subscribed, isGroup(m.Chat)))
	out.ParseMode = "html"
	out.ReplyMarkup = wotdButton(subscribed)
	_, err = n.send(out)
	return err
}

// HandleWordOfDayCallback toggles the subscription from the inline button,
// for the chat when pressed in a group, for the user otherwise.
func (n *Net) HandleWordOfDayCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	subscribe := cq.Data == "wotd_on"
	group := isGroup(cq.Message.Chat)

	if group {
		if err := n.repo.SetChatWordOfDaySubscription(ctx, cq.Message.Chat.ID, subscribe); err != nil {
			return fmt.Errorf("repo.SetChatWordOfDaySubscription: %w", err)
		}
	} else {
		if err := n.repo.StoreUser(ctx, int(cq.From.ID), cq.From.UserName); err != nil {
			return fmt.Errorf("repo.StoreUser: %w", err)
		}
		if err := n.repo.SetWordOfDaySubscription(ctx, cq.From.ID, subscribe); err != nil {
			return fmt.Errorf("repo.SetWordOfDaySubscription: %w", err)
		}
	}

	toast := WotdUnsubscribedToast
	if subscribe {
		toast = WotdSubscribedToast
	}
	if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, toast)); err != nil {
		n.log.WithError(err).Warn("failed to ack wotd callback")
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(cq.Message.Chat.ID, cq.Message.MessageID, wotdStatusText(subscribe, group), wotdButton(subscribe))
	edit.ParseMode = "html"
	_, err := n.send(edit)
	return err
}

func isGroup(chat *tgbotapi.Chat) bool {
	return chat != nil && (chat.Type == "group" || chat.Type == "supergroup")
}

func wotdStatusText(subscribed, group bool) string {
	switch {
	case group && subscribed:
		return WotdChatStatusOnText
	case group:
		return WotdChatStatusOffText
	case subscribed:
		return WotdStatusOnText
	default:
		return WotdStatusOffText
	}
}

func wotdButton(subscribed bool) tgbotapi.InlineKeyboardMarkup {
	label, data := WotdSubscribeButton, "wotd_on"
	if subscribed {
		label, data = WotdUnsubscribeButton, "wotd_off"
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(label, data)),
	)
}

// maybeSuggestWordOfDay sends the one-time subscription suggestion to engaged
// users (WotdNudgeMinLookups+ searches) who never opted in. Marked as sent
// before sending so a failure can never turn it into repeat nudging.
func (n *Net) maybeSuggestWordOfDay(ctx context.Context, chatID, userID int64) {
	nudged, err := n.repo.WasWordOfDayNudged(ctx, userID)
	if err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("wotd nudge: check failed")
		return
	}
	if nudged {
		return
	}
	subscribed, err := n.repo.IsWordOfDaySubscribed(ctx, userID)
	if err != nil || subscribed {
		return
	}
	lookups, err := n.repo.CountUserActivity(ctx, userID)
	if err != nil || lookups < WotdNudgeMinLookups {
		return
	}

	if err := n.repo.MarkWordOfDayNudged(ctx, userID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("wotd nudge: mark failed")
		return
	}
	msg := tgbotapi.NewMessage(chatID, WotdNudgeText)
	msg.ReplyMarkup = wotdButton(false)
	if _, err := n.send(msg); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("wotd nudge: send failed")
	}
}

// StartWordOfDayScheduler launches a background goroutine that pushes a random
// Chechen word to all subscribers once a day at WordOfDayHour (local time).
// It stops when ctx is cancelled.
func (n *Net) StartWordOfDayScheduler(ctx context.Context) {
	go func() {
		// A deploy around the send hour kills the in-memory timer; if the last
		// recorded broadcast is from an earlier day, today's was missed. An
		// absent record stays conservative — better to skip once than spam.
		if n.cache != nil {
			if last, err := n.cache.GetWordOfDayLastSent(ctx); err == nil && wordOfDayDue(time.Now(), last, WordOfDayHour) {
				n.log.Info("word of the day: missed today's send, catching up")
				n.sendWordOfDay(ctx)
			}
		}
		for {
			next := nextWordOfDayTime(time.Now(), WordOfDayHour)
			n.log.Infof("word of the day: next send at %s", next.Format(time.RFC3339))

			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				n.sendWordOfDay(ctx)
				n.resolveMissingWords(ctx)
			}
		}
	}()
}

// missingWordsRecheckLimit caps how many gap-list words the daily sweep
// re-checks against the API.
const missingWordsRecheckLimit = 50

// resolveMissingWords re-checks the most-wanted missing words once a day:
// dosham grows, and resolved entries should leave the report without waiting
// for a user to happen to re-search them.
func (n *Net) resolveMissingWords(ctx context.Context) {
	words, err := n.repo.TopMissingWords(ctx, missingWordsRecheckLimit)
	if err != nil {
		n.log.WithError(err).Warn("missing words recheck: list")
		return
	}
	resolved := 0
	for _, w := range words {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if n.business.RecheckTranslation(w.CleanWord) {
			if err := n.repo.ResolveMissingWord(ctx, w.CleanWord); err != nil {
				n.log.WithError(err).WithField("word", w.CleanWord).Warn("missing words recheck: resolve")
				continue
			}
			resolved++
		}
		time.Sleep(500 * time.Millisecond) // be kind to the dosham API
	}
	if resolved > 0 {
		n.log.Infof("missing words recheck: resolved %d of %d", resolved, len(words))
	}
}

// wordOfDayDue reports whether today's broadcast was missed: the send hour
// has passed and the last recorded send happened on an earlier day.
func wordOfDayDue(now time.Time, lastSent string, hour int) bool {
	return now.Hour() >= hour && lastSent != now.Format(time.DateOnly)
}

// nextWordOfDayTime returns the next occurrence of hour:00 in now's location,
// today if it is still ahead, otherwise tomorrow.
func nextWordOfDayTime(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// wotdDrawAttempts bounds how many redraws the picker spends avoiding a
// recently sent word before settling for a repeat.
const wotdDrawAttempts = 5

// pickFresh draws words until one is not in recent, falling back to the first
// draw — a repeat beats skipping the day entirely.
func pickFresh(recent []string, draw func() *models.RandomWord) *models.RandomWord {
	seen := make(map[string]bool, len(recent))
	for _, w := range recent {
		seen[strings.ToLower(w)] = true
	}

	var fallback *models.RandomWord
	for range wotdDrawAttempts {
		word := draw()
		if word == nil {
			break
		}
		if !seen[strings.ToLower(word.Chechen)] {
			return word
		}
		if fallback == nil {
			fallback = word
		}
	}
	return fallback
}

// usageExample mines the word's dictionary glosses for a usage example.
// Shared by the Word of the Day and /random cards.
func (n *Net) usageExample(chechen string) (string, bool) {
	// Enrichment only: an unreachable dictionary means the card ships without
	// an example, not that the card is wrong.
	pairs, err := n.business.Translate(chechen)
	if err != nil {
		return "", false
	}
	for i, p := range pairs {
		if i == 5 {
			break
		}
		// The lookup key is the Chechen word, so its glosses read Chechen first.
		if ex, ok := tools.FirstExample(p.Translate, chechen, true); ok {
			return ex, true
		}
	}
	return "", false
}

// sendWordOfDay fetches one random word and broadcasts it to every subscriber.
func (n *Net) sendWordOfDay(ctx context.Context) {
	subscribers, err := n.repo.ListWordOfDaySubscribers(ctx)
	if err != nil {
		n.log.WithError(err).Error("word of the day: list subscribers")
		return
	}
	chats, err := n.repo.ListWordOfDayChatIDs(ctx)
	if err != nil {
		n.log.WithError(err).Error("word of the day: list chats")
		return
	}
	if len(subscribers) == 0 && len(chats) == 0 {
		return
	}

	var recent []string
	if n.cache != nil {
		if r, err := n.cache.RecentWordsOfDay(ctx); err == nil {
			recent = r
		}
	}
	word := pickFresh(recent, func() *models.RandomWord {
		w, err := n.business.RandomWordFromAPI(ctx)
		if err != nil {
			return nil
		}
		return w
	})
	if word == nil {
		n.log.Warn("word of the day: no word available, skipping")
		return
	}
	if n.cache != nil {
		if err := n.cache.RememberWordOfDay(ctx, word.Chechen); err != nil {
			n.log.WithError(err).Warn("word of the day: remember recent word")
		}
	}

	text := fmt.Sprintf(
		WordOfDayFormat,
		tgbotapi.EscapeText(tgbotapi.ModeHTML, word.Chechen),
		tgbotapi.EscapeText(tgbotapi.ModeHTML, word.Russian),
	)
	// One dictionary and one grammar lookup for the whole broadcast, not per
	// subscriber: a real usage example turns the card from vocabulary into language.
	if ex, ok := n.usageExample(word.Chechen); ok {
		text += "\n\n" + fmt.Sprintf(WordOfDayExampleFormat, tgbotapi.EscapeText(tgbotapi.ModeHTML, ex))
	}
	if line := n.grammarHint(ctx, word.Chechen); line != "" {
		text += "\n\n" + line
	}
	text += "\n\n" + WordOfDayFooter
	n.log.Infof("word of the day: sending %q to %d subscribers and %d chats", word.Chechen, len(subscribers), len(chats))

	// Recorded before the sends: if the broadcast is cut partway, at-most-once
	// beats greeting the survivors twice tomorrow.
	if n.cache != nil {
		if err := n.cache.SetWordOfDayLastSent(ctx, time.Now().Format(time.DateOnly)); err != nil {
			n.log.WithError(err).Warn("word of the day: record last sent")
		}
	}

	// The daily word doubles as a doorway: one tap serves another word, and
	// the share button carries today's word (and the bot) into other chats.
	buttons := wordCardButtons(word.Chechen)

	for _, id := range subscribers {
		select {
		case <-ctx.Done():
			n.log.Info("word of the day: broadcast interrupted by shutdown")
			return
		default:
		}
		out := tgbotapi.NewMessage(id, text)
		out.ParseMode = "html"
		out.ReplyMarkup = buttons
		if _, err := n.send(out); err != nil {
			if n.isBlockedError(err) {
				if mErr := n.repo.MarkUserBlocked(ctx, id, "word_of_day"); mErr != nil {
					n.log.WithError(mErr).WithField("user_id", id).Warn("word of the day: mark blocked")
				}
			} else {
				n.log.WithError(err).WithField("user_id", id).Warn("word of the day: send failed")
			}
		}
		time.Sleep(BroadcastSendDelay)
	}

	// Group chats get the same card. A kicked bot reads as a blocked error —
	// drop the chat's subscription instead of failing every morning.
	for _, id := range chats {
		select {
		case <-ctx.Done():
			n.log.Info("word of the day: chat broadcast interrupted by shutdown")
			return
		default:
		}
		out := tgbotapi.NewMessage(id, text)
		out.ParseMode = "html"
		out.ReplyMarkup = buttons
		if _, err := n.send(out); err != nil {
			if n.isBlockedError(err) {
				if uErr := n.repo.SetChatWordOfDaySubscription(ctx, id, false); uErr != nil {
					n.log.WithError(uErr).WithField("chat_id", id).Warn("word of the day: unsubscribe chat")
				}
			} else {
				n.log.WithError(err).WithField("chat_id", id).Warn("word of the day: chat send failed")
			}
		}
		time.Sleep(BroadcastSendDelay)
	}
}

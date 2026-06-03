package net

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// HandleWordOfDay shows the user's Word of the Day subscription status with a
// button to opt in or out.
func (n *Net) HandleWordOfDay(ctx context.Context, update *tgbotapi.Update) error {
	msg := update.Message
	if err := n.repo.StoreUser(ctx, int(msg.From.ID), msg.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	subscribed, err := n.repo.IsWordOfDaySubscribed(ctx, msg.From.ID)
	if err != nil {
		return fmt.Errorf("repo.IsWordOfDaySubscribed: %w", err)
	}

	out := tgbotapi.NewMessage(msg.Chat.ID, wotdStatusText(subscribed))
	out.ParseMode = "html"
	out.ReplyMarkup = wotdButton(subscribed)
	_, err = n.bot.Send(out)
	return err
}

// HandleWordOfDayCallback toggles the subscription from the inline button.
func (n *Net) HandleWordOfDayCallback(ctx context.Context, update *tgbotapi.Update) error {
	cq := update.CallbackQuery
	subscribe := cq.Data == "wotd_on"

	if err := n.repo.StoreUser(ctx, int(cq.From.ID), cq.From.UserName); err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}
	if err := n.repo.SetWordOfDaySubscription(ctx, cq.From.ID, subscribe); err != nil {
		return fmt.Errorf("repo.SetWordOfDaySubscription: %w", err)
	}

	toast := WotdUnsubscribedToast
	if subscribe {
		toast = WotdSubscribedToast
	}
	if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, toast)); err != nil {
		n.log.WithError(err).Warn("failed to ack wotd callback")
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(cq.Message.Chat.ID, cq.Message.MessageID, wotdStatusText(subscribe), wotdButton(subscribe))
	edit.ParseMode = "html"
	_, err := n.bot.Send(edit)
	return err
}

func wotdStatusText(subscribed bool) string {
	if subscribed {
		return WotdStatusOnText
	}
	return WotdStatusOffText
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

// StartWordOfDayScheduler launches a background goroutine that pushes a random
// Chechen word to all subscribers once a day at WordOfDayHour (local time).
// It stops when ctx is cancelled.
func (n *Net) StartWordOfDayScheduler(ctx context.Context) {
	go func() {
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
			}
		}
	}()
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

// sendWordOfDay fetches one random word and broadcasts it to every subscriber.
func (n *Net) sendWordOfDay(ctx context.Context) {
	subscribers, err := n.repo.ListWordOfDaySubscribers(ctx)
	if err != nil {
		n.log.WithError(err).Error("word of the day: list subscribers")
		return
	}
	if len(subscribers) == 0 {
		return
	}

	word, err := n.business.RandomWordFromAPI(ctx)
	if err != nil || word == nil {
		n.log.WithError(err).Warn("word of the day: no word available, skipping")
		return
	}

	text := fmt.Sprintf(
		WordOfDayFormat,
		tgbotapi.EscapeText(tgbotapi.ModeHTML, word.Chechen),
		tgbotapi.EscapeText(tgbotapi.ModeHTML, word.Russian),
	)
	n.log.Infof("word of the day: sending %q to %d subscribers", word.Chechen, len(subscribers))

	for _, id := range subscribers {
		out := tgbotapi.NewMessage(id, text)
		out.ParseMode = "html"
		if _, err := n.bot.Send(out); err != nil {
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
}

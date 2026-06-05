package net

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StartStreakReminderScheduler launches the evening sweep that warns quiz
// players whose daily streak lapses at midnight. Same shape as the Word of the
// Day scheduler: catch up a missed sweep on startup, then fire daily.
func (n *Net) StartStreakReminderScheduler(ctx context.Context) {
	go func() {
		if n.cache != nil {
			if last, err := n.cache.GetStreakReminderLastSent(ctx); err == nil && wordOfDayDue(time.Now(), last, StreakReminderHour) {
				n.log.Info("streak reminder: missed today's sweep, catching up")
				n.sendStreakReminders(ctx)
			}
		}
		for {
			next := nextWordOfDayTime(time.Now(), StreakReminderHour)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				n.sendStreakReminders(ctx)
			}
		}
	}()
}

// sendStreakReminders messages everyone whose streak ends today unless they
// answer one more question. The answer button reuses the quiz_n callback, so
// saving the streak is a single tap away.
func (n *Net) sendStreakReminders(ctx context.Context) {
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	holders, err := n.repo.ListLapsingStreaks(ctx, yesterday)
	if err != nil {
		n.log.WithError(err).Error("streak reminder: list holders")
		return
	}

	// Recorded before the sends, mirroring the word-of-day at-most-once rule.
	if n.cache != nil {
		if err := n.cache.SetStreakReminderLastSent(ctx, time.Now().Format(time.DateOnly)); err != nil {
			n.log.WithError(err).Warn("streak reminder: record last sent")
		}
	}
	if len(holders) == 0 {
		return
	}
	n.log.Infof("streak reminder: nudging %d streak holders", len(holders))

	button := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(QuizNextButtonText, "quiz_n"),
		),
	)
	for _, h := range holders {
		select {
		case <-ctx.Done():
			n.log.Info("streak reminder: interrupted by shutdown")
			return
		default:
		}
		out := tgbotapi.NewMessage(h.UserID, fmt.Sprintf(StreakReminderFormat, h.Streak))
		out.ParseMode = "html"
		out.ReplyMarkup = button
		if _, err := n.bot.Send(out); err != nil {
			if n.isBlockedError(err) {
				if mErr := n.repo.MarkUserBlocked(ctx, h.UserID, "streak_reminder"); mErr != nil {
					n.log.WithError(mErr).WithField("user_id", h.UserID).Warn("streak reminder: mark blocked")
				}
			} else {
				n.log.WithError(err).WithField("user_id", h.UserID).Warn("streak reminder: send failed")
			}
		}
		time.Sleep(BroadcastSendDelay)
	}
}

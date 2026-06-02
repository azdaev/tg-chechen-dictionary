package net

import (
	"chetoru/internal/models"
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (n *Net) HandleStart(update *tgbotapi.Update) error {
	video := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FilePath(PathInlineVideo))
	video.Caption = StartMessageText

	_, err := n.bot.Send(video)
	return err
}

func (n *Net) HandleStats(ctx context.Context, update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	day := time.Now().Day()
	month := int(time.Now().Month())
	year := time.Now().Year()

	newMonthlyUsers, err := n.repo.CountNewMonthlyUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.CountNewMonthlyUsers: %w", err)
	}

	dailyActiveUsersLastMonth, err := n.repo.DailyActiveUsersInMonth(ctx, month, year, day)
	if err != nil {
		return fmt.Errorf("repo.DailyActiveUsersInMonth: %w", err)
	}

	monthlyActiveUsers, err := n.repo.MonthlyActiveUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.MonthlyActiveUsers: %w", err)
	}

	totalPairs, approvedPairs, err := n.repo.CountDictionaryPairs(ctx)
	if err != nil {
		return fmt.Errorf("repo.CountDictionaryPairs: %w", err)
	}

	missingCount, err := n.repo.CountMissingWords(ctx)
	if err != nil {
		return fmt.Errorf("repo.CountMissingWords: %w", err)
	}

	dictText := fmt.Sprintf(DictionaryStatsFormat, totalPairs, approvedPairs, missingCount)

	msg := tgbotapi.NewMessage(
		update.Message.Chat.ID,
		dictText+statsMessageText(newMonthlyUsers, monthlyActiveUsers, dailyActiveUsersLastMonth),
	)
	msg.ParseMode = "html"

	_, err = n.bot.Send(msg)
	return err
}

// HandleMissingWords lists the most-searched words that have no translation,
// helping maintainers prioritize which Chechen words to add to the dictionary.
func (n *Net) HandleMissingWords(ctx context.Context, update *tgbotapi.Update) error {
	if !n.isAdmin(update.Message.From.ID) {
		return nil
	}

	words, err := n.repo.TopMissingWords(ctx, MissingWordsLimit)
	if err != nil {
		return fmt.Errorf("repo.TopMissingWords: %w", err)
	}

	if len(words) == 0 {
		_, err = n.bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, MissingWordsEmpty))
		return err
	}

	text := MissingWordsHeader
	for i, w := range words {
		display := w.RawWord
		if display == "" {
			display = w.CleanWord
		}
		text += fmt.Sprintf(MissingWordRowFormat, i+1, tgbotapi.EscapeText(tgbotapi.ModeHTML, display), w.SearchCount)
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
	msg.ParseMode = "html"
	_, err = n.bot.Send(msg)
	return err
}

func statsMessageText(newMonthlyUsers int, monthlyActiveUsers int, dailyActivityInMonth []models.DailyActivity) string {
	messageText := fmt.Sprintf(StatsHeaderText, newMonthlyUsers, monthlyActiveUsers)
	for i, activity := range dailyActivityInMonth {
		day := i + 1
		messageText += fmt.Sprintf(DailyStatsFormat, day, activity.ActiveUsers, activity.Calls)
	}
	return messageText
}

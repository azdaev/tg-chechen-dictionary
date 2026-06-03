package net

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// quizLetters label the answer options. Indices map 1:1 to option/row order.
var quizLetters = []string{"А", "Б", "В", "Г", "Д", "Е"}

// HandleQuiz sends a fresh multiple-choice question. The correct answer index is
// encoded directly in each button's callback data, so no server-side state is
// needed to grade the answer.
func (n *Net) HandleQuiz(ctx context.Context, chatID int64) error {
	q, err := n.business.GenerateQuiz(ctx)
	if err != nil {
		n.log.WithError(err).Warn("GenerateQuiz failed")
		_, sErr := n.bot.Send(tgbotapi.NewMessage(chatID, QuizErrorText))
		return sErr
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(q.Options))
	for i, opt := range q.Options {
		letter := ""
		if i < len(quizLetters) {
			letter = quizLetters[i] + ". "
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				letter+opt,
				fmt.Sprintf("quiz_a_%d_%d", i, q.CorrectIdx),
			),
		))
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(QuizQuestionFormat, tgbotapi.EscapeText(tgbotapi.ModeHTML, q.Chechen)))
	msg.ParseMode = "html"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = n.bot.Send(msg)
	return err
}

// HandleQuizCallback grades an answer (or serves the next question). Callback
// data formats: "quiz_a_<chosen>_<correct>", "quiz_n" (next), "quiz_done" (noop).
func (n *Net) HandleQuizCallback(ctx context.Context, update *tgbotapi.Update) error {
	data := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	switch data {
	case "quiz_done":
		_, err := n.bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
		return err
	case "quiz_n":
		if _, err := n.bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "")); err != nil {
			n.log.WithError(err).Warn("failed to ack quiz next callback")
		}
		return n.HandleQuiz(ctx, chatID)
	}

	parts := strings.Split(data, "_") // [quiz a chosen correct]
	if len(parts) != 4 {
		return fmt.Errorf("invalid quiz callback data: %q", data)
	}
	chosenIdx, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("invalid quiz chosen index: %w", err)
	}
	correctIdx, err := strconv.Atoi(parts[3])
	if err != nil {
		return fmt.Errorf("invalid quiz correct index: %w", err)
	}

	correct := chosenIdx == correctIdx

	// Record the answer and fetch the running score for motivating feedback.
	userID := update.CallbackQuery.From.ID
	if err := n.repo.RecordQuizAnswer(ctx, userID, correct); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("RecordQuizAnswer failed")
	}

	toast := QuizWrongToast
	if correct {
		toast = QuizCorrectToast
	}
	if score, total, err := n.repo.GetQuizScore(ctx, userID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("GetQuizScore failed")
	} else if total > 0 {
		toast += fmt.Sprintf("  ·  Счёт: %d/%d (%d%%)", score, total, score*100/total)
	}

	if _, err := n.bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, toast)); err != nil {
		n.log.WithError(err).Warn("failed to ack quiz answer callback")
	}

	// Mark the result on the buttons and disable further answering.
	oldRows := update.CallbackQuery.Message.ReplyMarkup.InlineKeyboard
	newRows := make([][]tgbotapi.InlineKeyboardButton, 0, len(oldRows)+1)
	for i, row := range oldRows {
		if len(row) == 0 {
			continue
		}
		label := row[0].Text
		switch i {
		case correctIdx:
			label = "✅ " + label
		case chosenIdx:
			label = "❌ " + label
		}
		newRows = append(newRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "quiz_done"),
		))
	}
	newRows = append(newRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(QuizNextButtonText, "quiz_n"),
	))

	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, update.CallbackQuery.Message.MessageID, tgbotapi.NewInlineKeyboardMarkup(newRows...))
	if _, err := n.bot.Send(edit); err != nil {
		return fmt.Errorf("bot.Send edit: %w", err)
	}
	return nil
}

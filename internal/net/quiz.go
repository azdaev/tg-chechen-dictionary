package net

import (
	"chetoru/internal/models"
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// quizLetters label the answer options. Indices map 1:1 to option/row order.
var quizLetters = []string{"А", "Б", "В", "Г", "Д", "Е"}

// HandleQuiz sends a fresh question. In private chats it uses inline buttons
// with per-user persistent scoring; in groups it uses a native Telegram quiz
// poll so everyone can answer independently (inline buttons would let the first
// tapper lock the question for the whole group).
func (n *Net) HandleQuiz(ctx context.Context, chat *tgbotapi.Chat) error {
	q, err := n.business.GenerateQuiz(ctx)
	if err != nil {
		n.log.WithError(err).Warn("GenerateQuiz failed")
		_, sErr := n.bot.Send(tgbotapi.NewMessage(chat.ID, QuizErrorText))
		return sErr
	}

	if chat.Type == "group" || chat.Type == "supergroup" {
		return n.sendQuizPoll(ctx, chat.ID, q)
	}
	return n.sendQuizButtons(chat.ID, q)
}

// sendQuizPoll posts a native quiz poll — the idiomatic group experience. Each
// member answers on their own and Telegram reveals the correct option to them.
// The poll's correct option is cached by poll ID so answers can be graded into
// the leaderboard when poll_answer updates arrive.
func (n *Net) sendQuizPoll(ctx context.Context, chatID int64, q *models.QuizQuestion) error {
	question := fmt.Sprintf("🧠 Как переводится на русский: %s?", q.Prompt)
	if q.Reversed {
		question = fmt.Sprintf("🧠 Как сказать по-чеченски: %s?", q.Prompt)
	}
	poll := tgbotapi.NewPoll(chatID, question, q.Options...)
	poll.Type = "quiz"
	poll.CorrectOptionID = int64(q.CorrectIdx)
	poll.IsAnonymous = false
	// Let the group chain questions without retyping /quiz.
	poll.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(QuizNextButtonText, "quiz_n"),
		),
	)
	sent, err := n.bot.Send(poll)
	if err != nil {
		return err
	}
	if sent.Poll != nil {
		if err := n.cache.SetQuizPoll(ctx, sent.Poll.ID, q.CorrectIdx); err != nil {
			n.log.WithError(err).Warn("failed to cache quiz poll mapping")
		}
	}
	return nil
}

// HandlePollAnswer grades a group quiz-poll answer into the leaderboard. Votes on
// unknown/expired polls (or retracted votes) are ignored.
func (n *Net) HandlePollAnswer(ctx context.Context, pa *tgbotapi.PollAnswer) error {
	if pa == nil || len(pa.OptionIDs) == 0 {
		return nil
	}
	correctOption, err := n.cache.GetQuizPoll(ctx, pa.PollID)
	if err != nil {
		return nil // not one of our quiz polls, or it expired
	}
	correct := pa.OptionIDs[0] == correctOption
	if err := n.repo.RecordQuizAnswer(ctx, pa.User.ID, pa.User.UserName, pa.User.FirstName, correct); err != nil {
		return fmt.Errorf("RecordQuizAnswer (poll): %w", err)
	}
	return nil
}

// HandleTop renders the quiz leaderboard — friendly competition to encourage
// sustained vocabulary practice.
func (n *Net) HandleTop(ctx context.Context, chatID int64) error {
	scorers, err := n.repo.TopQuizScorers(ctx, QuizTopLimit)
	if err != nil {
		return fmt.Errorf("repo.TopQuizScorers: %w", err)
	}
	if len(scorers) == 0 {
		_, err = n.bot.Send(tgbotapi.NewMessage(chatID, QuizTopEmptyText))
		return err
	}

	var b strings.Builder
	b.WriteString(QuizTopHeader)
	for i, s := range scorers {
		name := scorerName(s)
		pct := 0
		if s.Total > 0 {
			pct = s.Correct * 100 / s.Total
		}
		fmt.Fprintf(&b, "%s <b>%s</b> — %d/%d (%d%%)", quizMedal(i), tgbotapi.EscapeText(tgbotapi.ModeHTML, name), s.Correct, s.Total, pct)
		if s.Streak >= 2 {
			fmt.Fprintf(&b, " 🔥%d", s.Streak)
		}
		b.WriteByte('\n')
	}

	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = "html"
	_, err = n.bot.Send(msg)
	return err
}

// scorerName picks the leaderboard display name: @username, then first name,
// then an anonymous placeholder.
func scorerName(s models.QuizScorer) string {
	switch {
	case s.Username != "":
		return s.Username
	case s.FirstName != "":
		return s.FirstName
	default:
		return "Аноним"
	}
}

// quizPromptFromMessage recovers the quizzed word from the question message:
// both question formats put the prompt alone on the last line.
func quizPromptFromMessage(text string) string {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		text = text[i+1:]
	}
	return strings.TrimSpace(text)
}

func quizMedal(rank int) string {
	switch rank {
	case 0:
		return "🥇"
	case 1:
		return "🥈"
	case 2:
		return "🥉"
	default:
		return fmt.Sprintf("%d.", rank+1)
	}
}

// sendQuizButtons posts the inline-button quiz used in private chats. The
// correct answer index is encoded in each button's callback data, so grading
// needs no server-side state.
func (n *Net) sendQuizButtons(chatID int64, q *models.QuizQuestion) error {
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

	format := QuizQuestionFormat
	if q.Reversed {
		format = QuizQuestionReverseFormat
	}
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(format, tgbotapi.EscapeText(tgbotapi.ModeHTML, q.Prompt)))
	msg.ParseMode = "html"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := n.bot.Send(msg)
	return err
}

// HandleQuizCallback grades an answer (or serves the next question). Callback
// data formats: "quiz_a_<chosen>_<correct>", "quiz_n" (next), "quiz_done" (noop).
func (n *Net) HandleQuizCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) error {
	data := cq.Data
	chatID := cq.Message.Chat.ID

	switch data {
	case "quiz_done":
		_, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, ""))
		return err
	case "quiz_n":
		if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
			n.log.WithError(err).Warn("failed to ack quiz next callback")
		}
		return n.HandleQuiz(ctx, cq.Message.Chat)
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
	userID := cq.From.ID
	if err := n.repo.RecordQuizAnswer(ctx, userID, cq.From.UserName, cq.From.FirstName, correct); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("RecordQuizAnswer failed")
	}

	toast := QuizWrongToast
	if correct {
		toast = QuizCorrectToast
	}
	if score, total, streak, err := n.repo.GetQuizScore(ctx, userID); err != nil {
		n.log.WithError(err).WithField("user_id", userID).Warn("GetQuizScore failed")
	} else if total > 0 {
		toast += fmt.Sprintf("  ·  Счёт: %d/%d (%d%%)", score, total, score*100/total)
		if streak >= 2 {
			toast += fmt.Sprintf("  ·  🔥 %d дн.", streak)
		}
	}

	if _, err := n.bot.Request(tgbotapi.NewCallback(cq.ID, toast)); err != nil {
		n.log.WithError(err).Warn("failed to ack quiz answer callback")
	}

	// Mark the result on the buttons and disable further answering.
	oldRows := cq.Message.ReplyMarkup.InlineKeyboard
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
	// Once answered, the word is worth a closer look — open its full
	// dictionary card via an inline query pre-filled with the prompt.
	if word := quizPromptFromMessage(cq.Message.Text); word != "" {
		newRows = append(newRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.InlineKeyboardButton{Text: QuizLookupButtonText, SwitchInlineQueryCurrentChat: &word},
		))
	}
	newRows = append(newRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(QuizNextButtonText, "quiz_n"),
	))

	edit := tgbotapi.NewEditMessageReplyMarkup(chatID, cq.Message.MessageID, tgbotapi.NewInlineKeyboardMarkup(newRows...))
	if _, err := n.bot.Send(edit); err != nil {
		return fmt.Errorf("bot.Send edit: %w", err)
	}
	return nil
}

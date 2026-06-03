package net

import (
	"chetoru/internal/models"
	"context"
	"fmt"
	"strconv"
	"strings"
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

	wotdSubscribers, err := n.repo.CountWordOfDaySubscribers(ctx)
	if err != nil {
		return fmt.Errorf("repo.CountWordOfDaySubscribers: %w", err)
	}

	quizPlayers, quizAnswers, quizCorrect, err := n.repo.CountQuizStats(ctx)
	if err != nil {
		return fmt.Errorf("repo.CountQuizStats: %w", err)
	}

	text := buildStatsMessage(statsData{
		month:           month,
		year:            year,
		newUsers:        newMonthlyUsers,
		activeUsers:     monthlyActiveUsers,
		totalPairs:      totalPairs,
		approvedPairs:   approvedPairs,
		missingWords:    missingCount,
		wotdSubscribers: wotdSubscribers,
		quizPlayers:     quizPlayers,
		quizAnswers:     quizAnswers,
		quizCorrect:     quizCorrect,
		daily:           dailyActiveUsersLastMonth,
	})

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, text)
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

var russianMonths = [...]string{
	"", "Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
}

// statsData bundles every figure shown in the /stats report.
type statsData struct {
	month, year     int
	newUsers        int
	activeUsers     int
	totalPairs      int
	approvedPairs   int
	missingWords    int
	wotdSubscribers int
	quizPlayers     int
	quizAnswers     int
	quizCorrect     int
	daily           []models.DailyActivity
}

// buildStatsMessage renders the admin /stats report as clean, Telegram-friendly
// HTML — emoji-marked sections and bold figures, no fixed-width/ASCII tables.
// The per-day breakdown lists only days that actually had activity, so the
// report stays compact instead of printing a row for every empty day.
func buildStatsMessage(d statsData) string {
	monthName := ""
	if d.month >= 1 && d.month <= 12 {
		monthName = russianMonths[d.month]
	}

	totalCalls, activeDays := 0, 0
	for _, day := range d.daily {
		totalCalls += day.Calls
		if day.ActiveUsers > 0 || day.Calls > 0 {
			activeDays++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📊 <b>Статистика · %s %d</b>\n\n", monthName, d.year)

	b.WriteString("👥 <b>Пользователи за месяц</b>\n")
	fmt.Fprintf(&b, "🆕 Новых: <b>%s</b>\n", formatThousands(d.newUsers))
	fmt.Fprintf(&b, "🟢 Активных: <b>%s</b>\n", formatThousands(d.activeUsers))
	fmt.Fprintf(&b, "🔁 Вызовов: <b>%s</b>\n\n", formatThousands(totalCalls))

	b.WriteString("📚 <b>Словарь</b>\n")
	fmt.Fprintf(&b, "📖 Всего пар: <b>%s</b>\n", formatThousands(d.totalPairs))
	fmt.Fprintf(&b, "✅ Проверено: <b>%s</b>\n", formatThousands(d.approvedPairs))
	fmt.Fprintf(&b, "🔍 Без перевода: <b>%s</b>\n\n", formatThousands(d.missingWords))

	b.WriteString("🎮 <b>Вовлечённость</b>\n")
	fmt.Fprintf(&b, "📖 Подписчиков на «Слово дня»: <b>%s</b>\n", formatThousands(d.wotdSubscribers))
	fmt.Fprintf(&b, "🧠 Игроков в викторину: <b>%s</b>\n", formatThousands(d.quizPlayers))
	quizPct := 0
	if d.quizAnswers > 0 {
		quizPct = d.quizCorrect * 100 / d.quizAnswers
	}
	fmt.Fprintf(&b, "✅ Ответов: <b>%s</b> (верных %s, %d%%)\n\n", formatThousands(d.quizAnswers), formatThousands(d.quizCorrect), quizPct)

	b.WriteString("📅 <b>По дням</b> <i>(день · 🟢 активных · 🔁 вызовов)</i>\n")
	if activeDays == 0 {
		b.WriteString("<i>Пока нет активности в этом месяце</i>")
		return b.String()
	}
	for i, day := range d.daily {
		if day.ActiveUsers == 0 && day.Calls == 0 {
			continue
		}
		fmt.Fprintf(&b, "<b>%d</b> · %d · %d\n", i+1, day.ActiveUsers, day.Calls)
	}

	return strings.TrimRight(b.String(), "\n")
}

// formatThousands formats an integer with spaces as thousands separators
// (e.g. 12345 -> "12 345") for readability in the stats report.
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(s[i])
	}
	return sign + b.String()
}

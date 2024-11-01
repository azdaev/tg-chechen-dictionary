package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"

	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

const (
	PathInlineVideo          = "internal/net/inline.mp4"
	MaxTranslations          = 4
	MoreTranslationsHelpText = `<i>Чтобы увидеть все переводы, воспользуйтесь инлайн режимом - 
напишите @chetoru_bot и слово, которое хотите перевести. 
Так вы увидите все варианты. Либо нажмите кнопку ниже, 
чтобы увидеть другие переводы</i>`

	StartMessageText  = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео"
	NoTranslationText = "К сожалению, нет перевода"
	MoreButtonText    = "Еще (%d)"
	StatsHeaderText   = `
<b>Статистика</b>

Новых пользователей за месяц: %d

Активных пользователей за месяц: %d

Уникальных пользователей на протяжении месяца:
<i>число месяца - кол-во уникальных пользователей - кол-во вызовов бота (включая инлайн)</i>
`
	DailyStatsFormat = "%d - %d - %d\n"
)

type Business interface {
	Translate(word string) []models.TranslationPairs
}

type Repository interface {
	StoreUser(ctx context.Context, userID int, username string) error
	StoreActivity(ctx context.Context, userID int, activityType models.ActivityType) error
	CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error)
	DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]models.DailyActivity, error)
	MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error)
}

type Net struct {
	log      *logrus.Logger
	repo     Repository
	business Business
	bot      *tgbotapi.BotAPI
}

func NewNet(log *logrus.Logger, repo Repository, bot *tgbotapi.BotAPI, business Business) *Net {
	return &Net{
		log:      log,
		repo:     repo,
		bot:      bot,
		business: business,
	}
}

func (s *Net) Start(ctx context.Context) {
	s.log.Info("starting service")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := s.bot.GetUpdatesChan(u)

	for update := range updates {
		// Обработка callback запросов
		if update.CallbackQuery != nil && strings.HasPrefix(update.CallbackQuery.Data, "more_") {
			err := s.HandleMoreTranslations(ctx, &update)
			if err != nil {
				s.log.WithError(err).Error("service.HandleMoreTranslations")
			}
			continue
		}

		// Обработка текстовых сообщений
		if update.Message != nil {
			if update.Message.Command() == "start" {
				err := s.HandleStart(&update)
				if err != nil {
					s.log.
						WithError(err).
						Error("service.HandleStart")
				}
				continue
			}

			if update.Message.Command() == "stats" {
				err := s.HandleStats(ctx, &update)
				if err != nil {
					s.log.
						WithError(err).
						Error("service.HandleStats")
				}
				continue
			}

			err := s.HandleText(ctx, &update)
			if err != nil {
				s.log.
					WithField("user_id", update.Message.From.ID).
					WithField("username", update.Message.From.UserName).
					WithField("message", update.Message.Text).
					WithError(err).
					Error("service.HandleText")
			}
			continue
		}

		// Обработка inline запросов
		if update.InlineQuery != nil && update.InlineQuery.Query != "" {
			err := s.HandleInline(ctx, &update)
			if err != nil {
				s.log.
					WithField("user_id", update.InlineQuery.From.ID).
					WithField("username", update.InlineQuery.From.UserName).
					WithField("message", update.InlineQuery.Query).
					WithError(err).
					Error("service.HandleInline")
			}
			continue
		}
	}
}

func (s *Net) HandleText(ctx context.Context, update *tgbotapi.Update) error {
	err := s.repo.StoreUser(ctx, int(update.Message.From.ID), update.Message.From.UserName)
	if err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	err = s.repo.StoreActivity(ctx, int(update.Message.From.ID), models.ActivityTypeText)
	if err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	m := update.Message
	translations := s.business.Translate(m.Text)
	if len(translations) == 0 {
		_, err = s.bot.Send(tgbotapi.NewMessage(m.Chat.ID, NoTranslationText))
		if err != nil {
			return fmt.Errorf("bot.Send: %w", err)
		}
		return nil
	}

	// Показываем только первые 4 перевода
	firstTranslations := translations
	if len(translations) > MaxTranslations {
		firstTranslations = translations[:MaxTranslations]
	}

	msg := tgbotapi.NewMessage(m.Chat.ID, formatTranslations(firstTranslations))
	msg.ParseMode = "html"

	if len(translations) > MaxTranslations {
		remainingCount := len(translations) - MaxTranslations
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, remainingCount),
					fmt.Sprintf("more_%s_4", m.Text),
				),
			),
		)
		msg.ReplyMarkup = keyboard

		msg.Text += "\n\n" + MoreTranslationsHelpText
	}

	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	return nil
}

func (s *Net) HandleInline(ctx context.Context, update *tgbotapi.Update) error {
	translations := s.business.Translate(update.InlineQuery.Query)

	articles := make([]interface{}, len(translations))

	for i := range articles {
		article := tgbotapi.NewInlineQueryResultArticle(update.InlineQuery.ID+strconv.Itoa(i), tools.Clean(translations[i].Original), "")
		article.Description = tools.Clean(translations[i].Translate)
		article.InputMessageContent = tgbotapi.InputTextMessageContent{
			Text:      fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate),
			ParseMode: "html",
		}

		articles[i] = article
	}

	inlineConf := tgbotapi.InlineConfig{
		InlineQueryID: update.InlineQuery.ID,
		IsPersonal:    true,
		CacheTime:     0,
		Results:       articles,
	}

	resp, err := s.bot.Request(inlineConf)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}
	if !resp.Ok {
		return fmt.Errorf("bot.Request: %s", resp.Description)
	}

	err = s.repo.StoreUser(ctx, int(update.InlineQuery.From.ID), update.InlineQuery.From.UserName)
	if err != nil {
		return fmt.Errorf("repo.StoreUser: %w", err)
	}

	err = s.repo.StoreActivity(ctx, int(update.InlineQuery.From.ID), models.ActivityTypeInline)
	if err != nil {
		return fmt.Errorf("repo.StoreActivity: %w", err)
	}

	return nil
}

func (s *Net) HandleStart(update *tgbotapi.Update) error {
	video := tgbotapi.NewVideo(update.Message.Chat.ID, tgbotapi.FilePath(PathInlineVideo))
	video.Caption = StartMessageText

	_, err := s.bot.Send(video)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	return nil
}

func (s *Net) HandleStats(ctx context.Context, update *tgbotapi.Update) error {
	if strconv.Itoa(int(update.Message.From.ID)) != os.Getenv("TG_ADMIN_ID") {
		return nil
	}

	day := time.Now().Day()
	month := int(time.Now().Month())
	year := time.Now().Year()
	newMonthlyUsers, err := s.repo.CountNewMonthlyUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.CountNewMonthlyUsers: %w", err)
	}

	dailyActiveUsersLastMonth, err := s.repo.DailyActiveUsersInMonth(ctx, month, year, day)
	if err != nil {
		return fmt.Errorf("repo.DailyActiveUsersInMonth: %w", err)
	}

	monthlyActiveUsers, err := s.repo.MonthlyActiveUsers(ctx, month, year)
	if err != nil {
		return fmt.Errorf("repo.MonthlyActiveUsers: %w", err)
	}

	msg := tgbotapi.NewMessage(
		update.Message.Chat.ID,
		statsMessageText(newMonthlyUsers, monthlyActiveUsers, dailyActiveUsersLastMonth),
	)
	msg.ParseMode = "html"

	_, err = s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	return nil
}

func statsMessageText(newMonthlyUsers int, monthlyActiveUsers int, dailyActivityInMonth []models.DailyActivity) string {
	messageText := fmt.Sprintf(StatsHeaderText, newMonthlyUsers, monthlyActiveUsers)

	for i, activity := range dailyActivityInMonth {
		day := i + 1
		messageText += fmt.Sprintf(DailyStatsFormat, day, activity.ActiveUsers, activity.Calls)
	}

	return messageText
}

func (s *Net) HandleMoreTranslations(ctx context.Context, update *tgbotapi.Update) error {
	parts := strings.Split(update.CallbackQuery.Data, "_")
	if len(parts) != 3 {
		return fmt.Errorf("invalid callback data format")
	}

	word := parts[1]                    // слово, которое нужно перевести
	offset, _ := strconv.Atoi(parts[2]) // номер первого перевода, который нужно показать

	translations := s.business.Translate(word)

	// Получаем следующие 4 перевода
	end := min(offset+4, len(translations))
	nextTranslations := translations[offset:end]

	msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, formatTranslations(nextTranslations))
	msg.ParseMode = "html"

	// Если есть еще переводы, добавляем новую кнопку "еще"
	if end < len(translations) {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf(MoreButtonText, len(translations)-end),
					fmt.Sprintf("more_%s_%d", word, end),
				),
			),
		)
		msg.ReplyMarkup = keyboard
	}

	_, err := s.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("bot.Send: %w", err)
	}

	// Отвечаем на callback query, чтобы убрать "часики" с кнопки
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	_, err = s.bot.Request(callback)
	if err != nil {
		return fmt.Errorf("bot.Request: %w", err)
	}

	return nil
}

// Вспомогательная функция для форматирования переводов
func formatTranslations(translations []models.TranslationPairs) string {
	var result string
	for _, t := range translations {
		result += fmt.Sprintf("<b>%s</b> - %s\n\n", t.Original, t.Translate)
	}
	return result
}

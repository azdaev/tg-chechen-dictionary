package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"

	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

const PathInlineVideo = "internal/service/inline.mp4"

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
		_, err = s.bot.Send(tgbotapi.NewMessage(m.Chat.ID, "К сожалению, нет перевода"))
		if err != nil {
			return fmt.Errorf("bot.Send: %w", err)
		}
	}

	for i := range translations {
		msg := tgbotapi.NewMessage(m.Chat.ID, fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate))
		msg.ParseMode = "html"
		_, err = s.bot.Send(msg)
		if err != nil {
			return fmt.Errorf("bot.Send: %w", err)
		}
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
	video.Caption = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео"

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
	messageText := fmt.Sprintf(`
<b>Статистика</b>

Новых пользователей за месяц: %d

Активных пользователей за месяц: %d

Уникальных пользователей на протяжении месяца:
<i>число месяца - кол-во уникальных пользователей - кол-во вызовов бота (включая инлайн)</i>

`, newMonthlyUsers, monthlyActiveUsers)

	for i, activity := range dailyActivityInMonth {
		day := i + 1
		messageText += fmt.Sprintf("%d - %d - %d\n", day, activity.ActiveUsers, activity.Calls)
	}

	return messageText
}

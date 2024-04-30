package service

import (
	entities "chetoru/internal/models"
	"chetoru/tools"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	bots "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
)

type Repository interface {
	StoreUser(ctx context.Context, userID int, username string) error
	StoreActivity(ctx context.Context, userID int, activityType entities.ActivityType) error
	CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error)
	DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]int, error)
	MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error)
}

type Service struct {
	log  *logrus.Logger
	repo Repository
	bot  *bots.Bot
}

func NewService(log *logrus.Logger, repo Repository, bot *bots.Bot) *Service {
	return &Service{
		log:  log,
		repo: repo,
		bot:  bot,
	}
}

func (s *Service) Start(ctx context.Context) {
	s.bot.RegisterHandler(
		bots.HandlerTypeMessageText,
		"/stats",
		bots.MatchTypeExact,
		s.HandleStats,
	)

	s.bot.RegisterHandlerMatchFunc(
		func(update *models.Update) bool {
			return update.Message.Text != ""
		},
		s.TextHandler,
	)

	s.bot.RegisterHandlerMatchFunc(
		func(update *models.Update) bool {
			return update.InlineQuery != nil
		},
		s.InlineHandler,
	)
	s.log.Info("starting service")
	s.bot.Start(ctx)
}

func (s *Service) TextHandler(ctx context.Context, bot *bots.Bot, update *models.Update) {
	err := s.repo.StoreUser(ctx, int(update.Message.From.ID), update.Message.From.Username)
	if err != nil {
		s.log.WithError(err).Error("error storing user")
		return
	}

	err = s.repo.StoreActivity(ctx, int(update.Message.From.ID), entities.ActivityTypeText)
	if err != nil {
		s.log.WithError(err).Error("error storing activity")
		return
	}

	m := update.Message
	translations := tools.Translate(m.Text)
	if len(translations) == 0 {
		_, err = bot.SendMessage(ctx, &bots.SendMessageParams{
			ChatID: m.Chat.ID,
			Text:   "К сожалению, нет перевода",
		})
		if err != nil {
			s.log.WithError(err).Error("error sending message")
			return
		}
	}

	for i := range translations {
		_, err = bot.SendMessage(ctx, &bots.SendMessageParams{
			ChatID:    m.Chat.ID,
			Text:      fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate),
			ParseMode: "html",
		})
		if err != nil {
			s.log.WithError(err).Error("error sending message")
			return
		}
	}
}

func (s *Service) InlineHandler(ctx context.Context, bot *bots.Bot, update *models.Update) {
	translations := tools.Translate(update.InlineQuery.Query)

	results := make([]models.InlineQueryResult, len(translations))

	for i := range results {
		results[i] = &models.InlineQueryResultArticle{
			ID:    strconv.Itoa(i),
			Title: tools.Clean(translations[i].Original),
			InputMessageContent: &models.InputTextMessageContent{
				MessageText: fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate),
				ParseMode:   "html",
			},
			Description: tools.Clean(translations[i].Translate),
		}
	}

	ok, err := bot.AnswerInlineQuery(ctx, &bots.AnswerInlineQueryParams{
		InlineQueryID: update.InlineQuery.ID,
		Results:       results,
		IsPersonal:    true,
	})
	if err != nil {
		s.log.WithError(err).Error("error answering inline query")
		return
	}
	if !ok {
		s.log.Error("error answering inline query")
		return
	}

	err = s.repo.StoreUser(ctx, int(update.Message.From.ID), update.Message.From.Username)
	if err != nil {
		s.log.WithError(err).Error("error sending message")
		return
	}

	err = s.repo.StoreActivity(ctx, int(update.Message.From.ID), entities.ActivityTypeInline)
	if err != nil {
		s.log.WithError(err).Error("error storing activity")
		return
	}
}

func (s *Service) HandleStats(ctx context.Context, bot *bots.Bot, update *models.Update) {
	if strconv.Itoa(int(update.Message.From.ID)) != os.Getenv("TG_ADMIN_ID") {
		return
	}

	day := time.Now().Day()
	month := int(time.Now().Month())
	year := time.Now().Year()
	newMonthlyUsers, err := s.repo.CountNewMonthlyUsers(ctx, month, year)
	if err != nil {
		s.log.WithError(err).Error("error counting new monthly users")
		return
	}

	dailyActiveUsersLastMonth, err := s.repo.DailyActiveUsersInMonth(ctx, month, year, day)
	if err != nil {
		s.log.WithError(err).Error("error counting daily active users")
		return
	}

	monthlyActiveUsers, err := s.repo.MonthlyActiveUsers(ctx, month, year)
	if err != nil {
		s.log.WithError(err).Error("error counting monthly active users")
		return
	}

	_, err = bot.SendMessage(ctx, &bots.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      tools.StatsMessageText(newMonthlyUsers, monthlyActiveUsers, dailyActiveUsersLastMonth),
		ParseMode: "html",
	})
}

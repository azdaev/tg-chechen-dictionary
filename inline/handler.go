package inline

import (
	"chetoru/tools"
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func Handle(bot *tgbotapi.BotAPI, update *tgbotapi.Update) error {
	translations := tools.Translate(update.InlineQuery.Query)

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

	_, err := bot.Request(inlineConf)

	return err
}

package main

import (
	"chetoru/inline"
	"chetoru/tools"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	bot, err := tgbotapi.NewBotAPI("5655516680:AAHaP2F42L3tpP4R_GRVJTWtMnzWiwZXN74") // create new bot
	if err != nil {
		panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		switch {
		case update.InlineQuery != nil:
			err := inline.Handle(bot, &update)
			if err != nil {
				log.Println(err)
			}
		case update.Message != nil:
			m := update.Message
			translations := tools.Translate(m.Text)
			if len(translations) == 0 {
				response := tgbotapi.NewMessage(m.Chat.ID, "К сожалению, нет перевода")
				bot.Send(response)
				continue
			}

			for i := range translations {
				response := tgbotapi.NewMessage(m.Chat.ID, fmt.Sprintf("<b>%s</b> - %s", translations[i].Original, translations[i].Translate))
				response.ParseMode = "html"
				bot.Send(response)
			}
		}
	}
}

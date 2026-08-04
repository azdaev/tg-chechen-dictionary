// User-facing strings and the numbers that tune them.
package net

import "time"

const (
	MaxTranslations         = 4
	InlineResultsLimit      = 50   // Telegram's hard cap on answerInlineQuery results
	InlineDiscoveryCount    = 3    // random words served for an empty inline query
	InlineDiscoveryCacheSec = 300  // short edge cache so the trio rotates
	InlineCacheTimeSeconds  = 3600 // Telegram-side cache for non-personal inline answers
	// No <i>: on a translation card italic marks a usage example and nothing
	// else, and this text sits right under one.
	MoreTranslationsHelpText = `Все варианты — в инлайн-режиме: наберите в любом чате @chetoru_bot и слово.`
	StartMessageText         = "Отправь мне слово на русском или чеченском, а я скину перевод. Ещё ты можешь пользоваться ботом в других переписках, как на видео.\n\n🎲 /random — случайное чеченское слово.\n🧠 /quiz — викторина: проверь, как хорошо ты знаешь чеченский.\n🏆 /top — рейтинг знатоков.\n👤 /me — мой прогресс.\n📖 /wotd — слово дня каждое утро.\n✍️ /check — проверить орфографию (или начни сообщение с точки).\n\nСловарные данные предоставлены проектом dosham.app"
	NoTranslationText        = "К сожалению, нет перевода"
	// Shown when the dictionary itself failed. Saying "нет перевода" there is a
	// lie, and it is the lie that also files the user's word as a vocabulary gap.
	DictionaryUnavailableText = "Словарь сейчас недоступен. Попробуйте через минуту."
	// Shown when the quota itself could not be read. A storage failure used to
	// take the paywall exit — telling the user their free checks had run out and
	// offering a subscription for what they already had.
	SpellcheckUnavailableText = "Проверка орфографии сейчас недоступна. Попробуйте через минуту."
	MissingWordRecordedText   = "Слово записано — такие пропуски мы разбираем и пополняем словарь."
	// Palochka Ӏ is in a third of Chechen headwords and on no keyboard. The bot
	// and the dictionary both accept the digit 1 in its place; people just do
	// not know that, so a Chechen-looking miss says so.
	PalochkaHintText        = "💡 Палочку Ӏ можно набрать цифрой 1: <code>г1ала</code> = гӏала."
	CheckSpellingButtonText = "✍️ Проверить орфографию"
	SuggestionsHeaderText   = "🔍 <b>Возможно, вы искали:</b>"
	// The card is headed by the entry that answered, which is not always the
	// word that was typed: «кемсана» is filed under «кемс», and Chechen marks
	// noun class on the verb, so «ваха» is filed under «даха». Without a line
	// saying so the reader is handed a different word and no reason for it.
	ResolvedQueryFormat       = "<i>по запросу «%s»:</i>"
	MissingWordsLimit         = 30
	MissingWordsHeader        = "<b>🔍 Слова без перевода</b>\n\n<i>Слова, которые искали пользователи, но в словаре не нашлось перевода. Это подсказывает, какие слова стоит добавить.</i>\n\n"
	MissingWordsEmpty         = "Пока нет слов без перевода 🎉"
	MissingWordRowFormat      = "%d. <b>%s</b> — %d раз\n"
	RandomWordFormat          = "🎲 <b>Случайное слово</b>\n\n<b>%s</b> — %s"
	RandomMoreButtonText      = "🎲 Ещё одно"
	ShareWordButtonText       = "📤 Поделиться"
	RandomEmptyText           = "Словарь пока пуст. Попробуйте перевести несколько слов, и они появятся здесь!"
	QuizQuestionFormat        = "🧠 <b>Викторина</b>\n\nКак переводится на русский?\n\n<b>%s</b>"
	QuizQuestionReverseFormat = "🧠 <b>Викторина</b>\n\nКак сказать по-чеченски?\n\n<b>%s</b>"
	QuizNextButtonText        = "➡️ Следующий вопрос"
	// The answer mapping for a group poll lives in Redis and can only be written
	// after the poll is sent. When that write fails the answers still look
	// graded to each member and reach nobody's score, so the chat is told.
	QuizNotScoredText      = "⚠️ Ответы на этот вопрос не попадут в рейтинг — не удалось его сохранить."
	QuizLookupButtonText   = "📖 Открыть в словаре"
	QuizCorrectToast       = "✅ Верно!"
	QuizWrongToast         = "❌ Неверно"
	QuizErrorText          = "Не удалось составить вопрос. Попробуйте /quiz ещё раз."
	QuizTopLimit           = 10
	QuizTopHeader          = "🏆 <b>Топ знатоков чеченского</b>\n<i>по количеству верных ответов в /quiz</i>\n\n"
	QuizTopEmptyText       = "Пока никто не набрал очков в /quiz. Стань первым! 🧠"
	WordOfDayHour          = 9 // local hour (container TZ is Europe/Moscow)
	WordOfDayFormat        = "📖 <b>Слово дня</b>\n\n<b>%s</b> — %s"
	WordOfDayExampleFormat = "✍️ <i>%s</i>"
	// No 🇨🇪: CE is unassigned in ISO 3166-1, so it is not a flag anywhere —
	// clients render two letter tiles. And no <i>: the card above already
	// spends italic on its usage example.
	WordOfDayFooter       = "Учите чеченский каждый день!"
	WotdStatusOnText      = "📖 <b>Слово дня</b>\n\nВы подписаны ✅ — каждый день в 9:00 будете получать новое чеченское слово."
	WotdStatusOffText     = "📖 <b>Слово дня</b>\n\nПодпишитесь, чтобы каждое утро получать новое чеченское слово и пополнять словарный запас."
	WotdChatStatusOnText  = "📖 <b>Слово дня</b>\n\nЭтот чат подписан ✅ — каждое утро в 9:00 сюда приходит новое чеченское слово."
	WotdChatStatusOffText = "📖 <b>Слово дня</b>\n\nПодпишите этот чат, чтобы каждое утро здесь появлялось новое чеченское слово."
	WotdSubscribeButton   = "🔔 Подписаться"
	WotdUnsubscribeButton = "🔕 Отписаться"
	WotdSubscribedToast   = "Вы подписались на слово дня! 🔔"
	WotdUnsubscribedToast = "Вы отписались от слова дня"
	WotdNudgeText         = "📖 Кстати! Каждое утро бот может присылать вам одно чеченское слово с переводом — маленький шаг к языку каждый день."
	WotdNudgeMinLookups   = 5
	DonationMessageFormat = "🌱 Чтобы наш проект мог продолжить работать, вы можете помочь нам"
	DefaultModerationChat = int64(-5204234916)
	BroadcastParseMode    = "html"
	// Shown when a broadcast could not start at all. The draft is kept, because
	// the alternative is an admin retyping it with no idea it was lost.
	BroadcastNotStartedText    = "Не удалось получить список получателей — рассылка не началась. Черновик сохранён, нажмите «Отправить» ещё раз."
	BroadcastSendDelay         = 100 * time.Millisecond
	StreakReminderHour         = 19 // local hour (container TZ is Europe/Moscow)
	StreakReminderFormat       = "🔥 Ваша серия — <b>%d дн.</b> Один вопрос сегодня, и она продолжится!"
	FreeSpellcheckLimit        = 5
	SubscriptionPriceKopecks   = 10000 // 100 RUB
	SubscriptionPriceFormatted = "100 ₽"
	SubscriptionDuration       = 30 * 24 * time.Hour // 30 days
)

package net

import (
	"chetoru/pkg/tools"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// send delivers a message and, if Telegram rejects our markup, resends it once
// as plain text. Every handler goes through here rather than bot.Send directly:
// the bot has dozens of HTML sends, the rejection is all-or-nothing, and the
// text most likely to carry a broken tag is the least trusted — formatPair
// renders stored AI output verbatim, so one unbalanced <b> from the model costs
// the user the whole answer. A broadcast turns that into everyone's answer.
func (n *Net) send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return sendWithRetry(n.bot.Send, c)
}

// sendWithRetry is send's logic without a bot attached, so it can be tested.
func sendWithRetry(send func(tgbotapi.Chattable) (tgbotapi.Message, error), c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, err := send(c)
	if !parseFailure(err) {
		return msg, err
	}
	plain, ok := stripFormatting(c)
	if !ok {
		return msg, err
	}
	return send(plain)
}

// answerInline delivers an inline answer with the same fallback. Telegram
// validates the whole result set at once, so a single malformed card blanks the
// picker for every result beside it.
func (n *Net) answerInline(conf tgbotapi.InlineConfig) error {
	resp, err := n.bot.Request(conf)
	if parseFailure(err) {
		if plain, ok := stripFormatting(conf); ok {
			resp, err = n.bot.Request(plain)
		}
	}
	if err != nil {
		return err
	}
	if !resp.Ok {
		return &tgbotapi.Error{Code: resp.ErrorCode, Message: resp.Description}
	}
	return nil
}

// parseFailure reports whether Telegram rejected our formatting, as opposed to
// anything else that can go wrong. Only this failure may be retried: resending
// after a timeout or a "bot was blocked" would deliver the message twice.
func parseFailure(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "can't parse")
}

// stripFormatting returns c with its markup removed and its parse mode cleared,
// or ok=false when there is no formatted text to strip. Clearing the parse mode
// alone is not enough — the user would then read the literal <b> and <i> that
// Telegram refused to render.
func stripFormatting(c tgbotapi.Chattable) (tgbotapi.Chattable, bool) {
	switch v := c.(type) {
	case tgbotapi.MessageConfig:
		if v.ParseMode == "" {
			return nil, false
		}
		v.ParseMode = ""
		v.Text = tools.StripTags(v.Text)
		return v, true
	case tgbotapi.EditMessageTextConfig:
		if v.ParseMode == "" {
			return nil, false
		}
		v.ParseMode = ""
		v.Text = tools.StripTags(v.Text)
		return v, true
	case tgbotapi.PhotoConfig:
		if v.ParseMode == "" {
			return nil, false
		}
		v.ParseMode = ""
		v.Caption = tools.StripTags(v.Caption)
		return v, true
	case tgbotapi.InlineConfig:
		results := make([]any, len(v.Results))
		stripped := false
		for i, r := range v.Results {
			article, ok := r.(tgbotapi.InlineQueryResultArticle)
			if !ok {
				results[i] = r
				continue
			}
			if content, ok := article.InputMessageContent.(tgbotapi.InputTextMessageContent); ok && content.ParseMode != "" {
				content.ParseMode = ""
				content.Text = tools.StripTags(content.Text)
				article.InputMessageContent = content
				stripped = true
			}
			results[i] = article
		}
		if !stripped {
			return nil, false
		}
		v.Results = results
		return v, true
	}
	return nil, false
}

// Bot API rich messages: sendRichMessage and its editMessageText counterpart.
package net

import (
	"chetoru/pkg/tools"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// richMessages gates the whole feature. Rich messages are new enough that how
// an old client renders one is unverified, so this ships off and gets turned on
// by the deploy, not by a release.
//
// ponytail: a process-wide env read, not a per-chat setting. A per-user opt-in
// if the rollout ever needs one.
var richMessages = os.Getenv("RICH_MESSAGES") == "1"

// richLimit is Telegram's cap on a rich message, 32768 characters. A card that
// somehow clears it falls back to the plain renderer rather than being cut:
// clampMessage closes <b> and <i> and knows nothing about a truncated <table>.
const richLimit = 32000

// sendRich posts a rich-HTML message. The v5 library predates the method, so it
// goes out through MakeRequest — the whole reason no library migration is
// needed for any of this.
func (n *Net) sendRich(chatID int64, body string, markup any) (tgbotapi.Message, error) {
	var msg tgbotapi.Message
	params, err := richParams(body, markup)
	if err != nil {
		return msg, err
	}
	params["chat_id"] = strconv.FormatInt(chatID, 10)
	resp, err := n.bot.MakeRequest("sendRichMessage", params)
	if err != nil {
		return msg, err
	}
	return msg, json.Unmarshal(resp.Result, &msg)
}

// editRich replaces a message's content with rich HTML. editMessageText takes
// rich_message instead of text, which is what lets the grammar card keep being
// grown into the translation rather than sent after it.
func (n *Net) editRich(chatID int64, messageID int, body string) error {
	params, err := richParams(body, nil)
	if err != nil {
		return err
	}
	params["chat_id"] = strconv.FormatInt(chatID, 10)
	params["message_id"] = strconv.Itoa(messageID)
	_, err = n.bot.MakeRequest("editMessageText", params)
	return err
}

// richParams builds the shared half of both calls. InputRichMessage takes
// exactly one of html, markdown or blocks; html is the one that keeps our
// existing renderer, tags and all.
func richParams(body string, markup any) (tgbotapi.Params, error) {
	rich, err := json.Marshal(map[string]string{"html": body})
	if err != nil {
		return nil, err
	}
	params := tgbotapi.Params{"rich_message": string(rich)}
	if markup != nil {
		if err := params.AddInterface("reply_markup", markup); err != nil {
			return nil, err
		}
	}
	return params, nil
}

// richUsable reports whether a built card should actually be sent as one.
func richUsable(body string) bool {
	return richMessages && body != "" && len(body) <= richLimit
}

// firstTagText returns the contents of the first <tag>…</tag> in s.
func firstTagText(s, tag string) (string, bool) {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// cardBuilder glues a message together in whichever dialect it will be sent in.
// The two differ only in how pieces are separated: a plain message uses blank
// lines, a rich one uses block tags. Which pieces there are, and in what order,
// is one decision made once — the reason this is a builder and not two copies
// of the assembly.
type cardBuilder struct {
	rich  bool
	parts []string
}

// body takes the rendered card, already in this builder's dialect.
func (c *cardBuilder) body(s string) { c.push(s, "") }

// line takes a piece of inline HTML — the «рядом» line, a suggestion list.
func (c *cardBuilder) line(s string) { c.push(s, "p") }

// quote takes a remark the bot makes about the lookup rather than a piece of
// the dictionary entry: «по запросу «ваха»», the palochka hint. On a plain card
// those are italic lines indistinguishable from a usage example.
func (c *cardBuilder) quote(s string) { c.push(s, "blockquote") }

func (c *cardBuilder) push(s, tag string) {
	if s == "" {
		return
	}
	if c.rich && tag != "" {
		s = "<" + tag + ">" + s + "</" + tag + ">"
	}
	c.parts = append(c.parts, s)
}

func (c *cardBuilder) String() string {
	if !c.rich {
		return strings.Join(c.parts, "\n\n")
	}
	return strings.Join(c.parts, "") + tools.RichFooter
}

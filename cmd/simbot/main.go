// simbot runs the whole bot locally with a terminal in place of Telegram.
//
// Everything below net.Start is the real thing — the same handlers, the same
// cards, the same buttons — and only the transport is faked: an http.Client
// that answers getUpdates from what you type and renders sendMessage to the
// screen. So what the terminal shows is what a user would see, formatting and
// keyboard included, without a token, a chat, or a deploy.
//
//	go run ./cmd/simbot          # ./sim.db, no Redis, no AI
//	go run ./cmd/simbot -db :memory:
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chetoru/internal/business"
	"chetoru/internal/cache"
	"chetoru/internal/net"
	"chetoru/internal/repository"
	"chetoru/migrations"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

const (
	simUserID = 424242
	simChatID = 424242
	// quiet is how long the terminal waits for the bot to stop talking before
	// printing the next prompt. Replies are asynchronous — the grammar card is
	// edited in a goroutine after the translation is sent.
	quiet    = 350 * time.Millisecond
	deadline = 20 * time.Second
)

func main() {
	dbPath := flag.String("db", "./sim.db", "sqlite file for the simulated bot")
	redis := flag.String("redis", "", "redis address; empty means no cache")
	verbose := flag.Bool("v", false, "show the bot's own logs")
	flag.Parse()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	if *verbose {
		log.SetLevel(logrus.DebugLevel)
	}

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	migrations.Quiet()
	if err := migrations.Up(db); err != nil {
		fatal(err)
	}
	repo := repository.NewRepository(db)

	// An unreachable address is a cache that always misses, which is what a run
	// without Redis should be — not a nil pointer the business layer must guard.
	addr := *redis
	if addr == "" {
		addr = "127.0.0.1:1"
	}
	redisCache := cache.NewCache(addr, "")

	tg := newFakeTelegram()
	bot, err := tgbotapi.NewBotAPIWithClient("sim", tgbotapi.APIEndpoint, tg)
	if err != nil {
		fatal(err)
	}

	translator := business.NewBusiness(redisCache, repo, nil, log)
	translator.SetFoldedReady()
	service := net.NewNet(log, repo, bot, translator, redisCache, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.Start(ctx)

	banner(*dbPath)
	repl(tg)

	cancel()
	service.WaitBackground()
	translator.WaitBackground()
}

func banner(dbPath string) {
	fmt.Printf("%schetoru%s — локальный Telegram. База: %s\n", bold, reset, dbPath)
	fmt.Printf("%sслово или /команда — отправить; :1 — нажать кнопку; :q — выход%s\n\n", dim, reset)
}

// repl turns typed lines into updates and waits for the bot to finish talking.
func repl(tg *fakeTelegram) {
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%s›%s ", blue, reset)
		if !in.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(in.Text())
		switch {
		case line == "":
			continue
		case line == ":q" || line == ":quit":
			return
		case strings.HasPrefix(line, ":"):
			data, ok := tg.button(strings.TrimPrefix(line, ":"))
			if !ok {
				fmt.Printf("%s  нет такой кнопки%s\n", dim, reset)
				continue
			}
			said := tg.said()
			tg.push(callbackUpdate(data, tg.lastMessageID()))
			tg.waitQuiet(said)
		default:
			said := tg.said()
			tg.push(textUpdate(line))
			tg.waitQuiet(said)
		}
	}
}

func textUpdate(text string) tgbotapi.Update {
	m := &tgbotapi.Message{
		From: &tgbotapi.User{ID: simUserID, UserName: "sim", FirstName: "Сим"},
		Chat: &tgbotapi.Chat{ID: simChatID, Type: "private"},
		Text: text,
		Date: int(time.Now().Unix()),
	}
	// tgbotapi reads commands off the entity list, not off the leading slash.
	if strings.HasPrefix(text, "/") {
		m.Entities = []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(strings.Fields(text)[0])}}
	}
	return tgbotapi.Update{Message: m}
}

func callbackUpdate(data string, messageID int) tgbotapi.Update {
	return tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:   fmt.Sprintf("cb%d", time.Now().UnixNano()),
		From: &tgbotapi.User{ID: simUserID, UserName: "sim", FirstName: "Сим"},
		Message: &tgbotapi.Message{
			MessageID: messageID,
			Chat:      &tgbotapi.Chat{ID: simChatID, Type: "private"},
		},
		Data: data,
	}}
}

// fakeTelegram is the Bot API as far as tgbotapi is concerned: a client whose
// getUpdates returns what was typed and whose sends print to the screen.
type fakeTelegram struct {
	pending chan tgbotapi.Update

	mu       sync.Mutex
	buttons  map[string]string // index shown → callback data
	lastMsg  int
	updateID int
	msgID    int

	lastOutput atomic.Int64 // unix nanos of the last line printed
	messages   atomic.Int64 // how many messages have been drawn
}

func newFakeTelegram() *fakeTelegram {
	return &fakeTelegram{pending: make(chan tgbotapi.Update, 8), buttons: map[string]string{}}
}

func (f *fakeTelegram) push(u tgbotapi.Update) {
	f.lastOutput.Store(time.Now().UnixNano())
	f.pending <- u
}

func (f *fakeTelegram) button(i string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.buttons[i]
	return data, ok
}

func (f *fakeTelegram) lastMessageID() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastMsg
}

func (f *fakeTelegram) said() int64 { return f.messages.Load() }

// waitQuiet returns once the bot has answered and then gone silent. Waiting for
// silence alone is not enough: a lookup that has to ask dosham says nothing for
// a second, and the prompt would come back before the card did.
func (f *fakeTelegram) waitQuiet(before int64) {
	start := time.Now()
	for time.Since(start) < deadline {
		time.Sleep(50 * time.Millisecond)
		if f.messages.Load() > before && time.Since(time.Unix(0, f.lastOutput.Load())) > quiet {
			return
		}
	}
}

func (f *fakeTelegram) Do(req *http.Request) (*http.Response, error) {
	method := path.Base(req.URL.Path)
	params := readParams(req)

	switch method {
	case "getMe":
		return reply(`{"id":1,"is_bot":true,"first_name":"chetoru","username":"chetoru_sim"}`)

	case "getUpdates":
		return f.serveUpdates()

	case "sendMessage":
		return f.serveMessage("", params)

	case "sendPoll":
		return f.servePoll(params)

	case "editMessageText":
		return f.serveMessage("правка", params)

	case "sendPhoto", "sendVideo", "sendAnimation", "sendDocument":
		return f.serveMessage(strings.ToLower(strings.TrimPrefix(method, "send")), params)

	default:
		// answerCallbackQuery, setMyCommands, sendChatAction, deleteMessage —
		// nothing to show, and the handlers only check the error.
		return reply(`true`)
	}
}

// readParams reads a Bot API call's fields. Plain calls post a form; anything
// carrying a file — /start sends its welcome as a video caption — posts
// multipart, and reading only the form left that message blank.
func readParams(req *http.Request) url.Values {
	if req.Body == nil {
		return url.Values{}
	}
	body, _ := io.ReadAll(req.Body)
	ct, attrs, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if ct != "multipart/form-data" {
		params, _ := url.ParseQuery(string(body))
		return params
	}
	params := url.Values{}
	r := multipart.NewReader(strings.NewReader(string(body)), attrs["boundary"])
	for {
		part, err := r.NextPart()
		if err != nil {
			return params
		}
		if part.FileName() == "" {
			value, _ := io.ReadAll(part)
			params.Set(part.FormName(), string(value))
		}
	}
}

func (f *fakeTelegram) serveUpdates() (*http.Response, error) {
	select {
	case u := <-f.pending:
		f.mu.Lock()
		f.updateID++
		u.UpdateID = f.updateID
		if u.Message != nil {
			u.Message.MessageID = f.updateID
		}
		f.mu.Unlock()
		out, _ := json.Marshal([]tgbotapi.Update{u})
		return reply(string(out))
	case <-time.After(300 * time.Millisecond):
		return reply(`[]`)
	}
}

func (f *fakeTelegram) serveMessage(kind string, params url.Values) (*http.Response, error) {
	f.mu.Lock()
	f.msgID++
	id := f.msgID
	f.lastMsg = id
	f.mu.Unlock()

	text := params.Get("text")
	if text == "" {
		text = params.Get("caption")
	}
	if text != "" {
		f.print(f.render(kind, id, text, params.Get("reply_markup")))
	}
	out, _ := json.Marshal(tgbotapi.Message{
		MessageID: id,
		Chat:      &tgbotapi.Chat{ID: simChatID, Type: "private"},
		Text:      params.Get("text"),
	})
	return reply(string(out))
}

func (f *fakeTelegram) servePoll(params url.Values) (*http.Response, error) {
	var options []string
	_ = json.Unmarshal([]byte(params.Get("options")), &options)
	lines := []string{header("опрос", 0) + " " + bold + params.Get("question") + reset}
	for i, o := range options {
		lines = append(lines, fmt.Sprintf("  %s%d.%s %s", dim, i+1, reset, o))
	}
	f.print(strings.Join(lines, "\n"))
	return f.serveMessage("", url.Values{})
}

// render lays one message out the way Telegram would: formatting applied,
// buttons under the text, each numbered so it can be pressed.
func (f *fakeTelegram) render(kind string, id int, text, markup string) string {
	lines := []string{header(kind, id)}
	for _, l := range strings.Split(htmlToTerm(text), "\n") {
		lines = append(lines, "  "+l)
	}
	if row := f.renderButtons(markup); row != "" {
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func header(kind string, id int) string {
	label := "бот"
	switch kind {
	case "":
	case "правка":
		label += " ✎ правка"
	default:
		label += " · " + kind
	}
	if id > 0 {
		label += fmt.Sprintf(" #%d", id)
	}
	return dim + "▌ " + label + reset
}

type inlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
	URL  string `json:"url"`
}

// renderButtons numbers the keyboard and remembers what each number sends.
// Numbering restarts with every message, so :1 always means "the first button
// of whatever the bot just said" — the terminal's version of tapping it.
func (f *fakeTelegram) renderButtons(markup string) string {
	if markup == "" {
		return ""
	}
	var m struct {
		Inline [][]inlineButton `json:"inline_keyboard"`
	}
	if err := json.Unmarshal([]byte(markup), &m); err != nil || len(m.Inline) == 0 {
		return ""
	}

	f.mu.Lock()
	f.buttons = map[string]string{}
	n := 0
	var rows []string
	for _, row := range m.Inline {
		var cells []string
		for _, b := range row {
			n++
			key := fmt.Sprint(n)
			label := fmt.Sprintf("%s[:%s]%s %s", blue, key, reset, b.Text)
			switch {
			case b.Data != "":
				f.buttons[key] = b.Data
			case b.URL != "":
				label += dim + " → " + b.URL + reset
			default:
				// switch_inline_query and friends: Telegram would hand the tap
				// to another chat, so there is nothing here to press.
				label = fmt.Sprintf("%s[ · ] %s%s", dim, b.Text, reset)
			}
			cells = append(cells, label)
		}
		rows = append(rows, "  "+strings.Join(cells, "   "))
	}
	f.mu.Unlock()
	return strings.Join(rows, "\n")
}

func (f *fakeTelegram) print(s string) {
	fmt.Println(s)
	f.messages.Add(1)
	f.lastOutput.Store(time.Now().UnixNano())
}

func reply(result string) (*http.Response, error) {
	body := `{"ok":true,"result":` + result + `}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// Terminal equivalents of the tags Telegram renders.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	italic = "\033[3m"
	under  = "\033[4m"
	strike = "\033[9m"
	blue   = "\033[34m"
)

var tagRe = regexp.MustCompile(`</?([a-zA-Z]+)[^>]*>`)

// htmlToTerm renders Telegram's HTML subset with ANSI. Nesting is handled by
// reprinting the open tags after every close: a terminal has no closing code
// per attribute, only a reset.
func htmlToTerm(s string) string {
	var out strings.Builder
	var open []string
	at := 0
	for _, m := range tagRe.FindAllStringSubmatchIndex(s, -1) {
		out.WriteString(html.UnescapeString(s[at:m[0]]))
		at = m[1]
		tag := strings.ToLower(s[m[2]:m[3]])
		code, known := ansiFor(tag)
		if !known {
			continue
		}
		if strings.HasPrefix(s[m[0]:m[1]], "</") {
			for i := len(open) - 1; i >= 0; i-- {
				if open[i] == code {
					open = append(open[:i], open[i+1:]...)
					break
				}
			}
			out.WriteString(reset + strings.Join(open, ""))
			continue
		}
		open = append(open, code)
		out.WriteString(code)
	}
	out.WriteString(html.UnescapeString(s[at:]))
	if len(open) > 0 {
		out.WriteString(reset)
	}
	return out.String()
}

func ansiFor(tag string) (string, bool) {
	switch tag {
	case "b", "strong":
		return bold, true
	case "i", "em":
		return italic, true
	case "u", "ins":
		return under, true
	case "s", "strike", "del":
		return strike, true
	case "code", "pre":
		return dim, true
	case "a":
		return under, true
	}
	return "", false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "simbot:", err)
	os.Exit(1)
}

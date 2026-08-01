package net

import (
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// recordingSender captures what was sent and fails the first attempt with a
// configurable error.
type recordingSender struct {
	sent     []tgbotapi.Chattable
	failWith error
}

func (s *recordingSender) send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.sent = append(s.sent, c)
	if len(s.sent) == 1 && s.failWith != nil {
		return tgbotapi.Message{}, s.failWith
	}
	return tgbotapi.Message{}, nil
}

func htmlMessage(text string) tgbotapi.MessageConfig {
	m := tgbotapi.NewMessage(1, text)
	m.ParseMode = "html"
	return m
}

func TestSendWithRetry_ResendsPlainOnParseError(t *testing.T) {
	s := &recordingSender{failWith: &tgbotapi.Error{
		Code:    400,
		Message: "Bad Request: can't parse entities: Unclosed start tag \"b\"",
	}}

	if _, err := sendWithRetry(s.send, htmlMessage("<b>дитт</b> — дерево\n• пример")); err != nil {
		t.Fatalf("retry should have succeeded: %v", err)
	}
	if len(s.sent) != 2 {
		t.Fatalf("sent %d messages, want the original plus one retry", len(s.sent))
	}

	retry, ok := s.sent[1].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("retry is %T, want a MessageConfig", s.sent[1])
	}
	if retry.ParseMode != "" {
		t.Errorf("retry kept ParseMode %q — Telegram would reject it again", retry.ParseMode)
	}
	// Clearing the parse mode alone would show the user literal <b> tags.
	if strings.ContainsAny(retry.Text, "<>") {
		t.Errorf("retry still carries markup: %q", retry.Text)
	}
	// The card's shape survives: only tags go, not the line breaks.
	if !strings.Contains(retry.Text, "дитт — дерево\n• пример") {
		t.Errorf("retry lost the line structure: %q", retry.Text)
	}
}

func TestSendWithRetry_DoesNotResendOnOtherErrors(t *testing.T) {
	// Resending after a timeout or a block delivers the message twice; only a
	// rejected-markup failure is safe to repeat.
	for _, failure := range []error{
		errors.New("Post \"https://api.telegram.org\": dial tcp: i/o timeout"),
		&tgbotapi.Error{Code: 403, Message: "Forbidden: bot was blocked by the user"},
		&tgbotapi.Error{Code: 400, Message: "Bad Request: message is too long"},
	} {
		s := &recordingSender{failWith: failure}
		if _, err := sendWithRetry(s.send, htmlMessage("<b>дитт</b>")); err == nil {
			t.Errorf("%v: error was swallowed", failure)
		}
		if len(s.sent) != 1 {
			t.Errorf("%v: sent %d messages, want no retry", failure, len(s.sent))
		}
	}
}

func TestSendWithRetry_UnformattedMessageIsNotRetried(t *testing.T) {
	// Nothing to strip means the retry would be byte-identical, so it can only
	// duplicate the message.
	s := &recordingSender{failWith: &tgbotapi.Error{Code: 400, Message: "Bad Request: can't parse entities"}}
	if _, err := sendWithRetry(s.send, tgbotapi.NewMessage(1, "простой текст")); err == nil {
		t.Fatal("error was swallowed")
	}
	if len(s.sent) != 1 {
		t.Fatalf("sent %d messages, want no retry", len(s.sent))
	}
}

func TestStripFormatting_InlineAnswer(t *testing.T) {
	// Telegram validates an inline answer as a whole, so one bad card blanks
	// the picker — the fallback has to strip every article, not just the
	// offending one, since it never says which.
	article := tgbotapi.NewInlineQueryResultArticle("1", "Дитт", "")
	article.InputMessageContent = tgbotapi.InputTextMessageContent{
		Text:      "<b>Дитт</b> — дерево",
		ParseMode: "html",
	}
	conf := tgbotapi.InlineConfig{InlineQueryID: "q", Results: []any{article}}

	plain, ok := stripFormatting(conf)
	if !ok {
		t.Fatal("inline answer with HTML content must be strippable")
	}
	got := plain.(tgbotapi.InlineConfig).Results[0].(tgbotapi.InlineQueryResultArticle)
	content := got.InputMessageContent.(tgbotapi.InputTextMessageContent)
	if content.ParseMode != "" {
		t.Errorf("retry kept ParseMode %q", content.ParseMode)
	}
	if content.Text != "Дитт — дерево" {
		t.Errorf("content = %q, want the tags stripped", content.Text)
	}
	if got.Title != "Дитт" {
		t.Errorf("title was disturbed: %q", got.Title)
	}

	// An answer that never asked for markup has nothing to fall back to.
	bare := tgbotapi.NewInlineQueryResultArticle("1", "Дитт", "дерево")
	if _, ok := stripFormatting(tgbotapi.InlineConfig{Results: []any{bare}}); ok {
		t.Error("unformatted inline answer should not be retried")
	}
}

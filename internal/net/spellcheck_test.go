package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type countingAI struct {
	calls int
	err   error
}

func (a *countingAI) SpellCheck(_ context.Context, _ string) (*ai.SpellCheckResult, error) {
	a.calls++
	if a.err != nil {
		return nil, a.err
	}
	return &ai.SpellCheckResult{NoErrors: true}, nil
}

func TestSpellcheck_CacheUnavailableFallsThroughToAI(t *testing.T) {
	// An unreachable Redis must degrade to a plain AI call, not an error.
	a := &countingAI{}
	n := &Net{log: logrus.New(), ai: a, cache: cache.NewCache("127.0.0.1:1", "")}

	result, err := n.spellcheck(context.Background(), "дала безам бу")
	if err != nil {
		t.Fatalf("spellcheck() error = %v", err)
	}
	if !result.NoErrors {
		t.Fatalf("result = %+v, want NoErrors", result)
	}
	if a.calls != 1 {
		t.Fatalf("AI calls = %d, want 1", a.calls)
	}

	a.err = errors.New("openrouter down")
	if _, err := n.spellcheck(context.Background(), "дала безам бу"); err == nil {
		t.Fatal("AI failure must propagate")
	}
}

func TestInlineSpellDebounce_SupersededQuerySkipped(t *testing.T) {
	n := &Net{inlineSpellLatest: make(map[int64]string)}

	n.noteInlineSpellQuery(1, "q1")
	n.noteInlineSpellQuery(1, "q2") // user kept typing
	n.noteInlineSpellQuery(2, "other-user")

	if n.isLatestInlineSpellQuery(1, "q1") {
		t.Fatal("superseded query must be skipped")
	}
	if !n.isLatestInlineSpellQuery(1, "q2") {
		t.Fatal("latest query must run")
	}
	if n.isLatestInlineSpellQuery(1, "q2") {
		t.Fatal("settled query must not run twice")
	}
	if !n.isLatestInlineSpellQuery(2, "other-user") {
		t.Fatal("users must be debounced independently")
	}
}

func TestCheckTarget(t *testing.T) {
	command := func(text string) *tgbotapi.Message {
		return &tgbotapi.Message{
			Text:     text,
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 6}},
		}
	}
	if got := checkTarget(command("/check")); got != "" {
		t.Errorf("a bare /check spellchecks %q, want the usage text instead", got)
	}
	if got := checkTarget(command("/check дала безам бу")); got != "дала безам бу" {
		t.Errorf("checkTarget = %q, want the arguments", got)
	}
	if got := checkTarget(&tgbotapi.Message{Text: ".дала безам бу"}); got != ".дала безам бу" {
		t.Errorf("dot-prefix mode lost its text: %q", got)
	}
}

package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"context"
	"errors"
	"strings"
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

// Both metered callers used to log a storage error and then take the paywall
// branch, so a database outage told the user their five free checks were gone
// and offered a subscription for what they still had.
func TestSpellcheckAccess_FailureIsNotExhaustion(t *testing.T) {
	if got := spellcheckAccessFor(false, errors.New("database is locked")); got != spellcheckUnreadable {
		t.Errorf("an unreadable quota reads as %v, want spellcheckUnreadable", got)
	}
	// The same error with allowed=true is still unreadable: canUseSpellcheck
	// returns false on error today, and a future true must not open the gate.
	if got := spellcheckAccessFor(true, errors.New("database is locked")); got != spellcheckUnreadable {
		t.Errorf("an errored check reads as %v, want spellcheckUnreadable", got)
	}
	if got := spellcheckAccessFor(false, nil); got != spellcheckPaywalled {
		t.Errorf("a genuinely exhausted quota reads as %v, want spellcheckPaywalled", got)
	}
	if got := spellcheckAccessFor(true, nil); got != spellcheckAllowed {
		t.Errorf("a user with checks left reads as %v, want spellcheckAllowed", got)
	}
}

// A correction the reader has to find by comparing two spellings letter by
// letter is barely a correction. The reply carried the fixed text and no
// markup at all — not even a parse mode — so the one card where bold would
// carry information could not use it.
func TestMarkCorrections(t *testing.T) {
	cases := []struct{ name, typed, corrected, want string }{
		{"одно слово исправлено", "г1ала чохь", "гӏала чохь", "<b>гӏала</b> чохь"},
		{"ничего не изменилось", "гӏала", "гӏала", "гӏала"},
		{"регистр — не правка", "Гӏала", "гӏала", "гӏала"},
		{"пунктуация и пробелы на месте", "со ваха,  цига", "со вахна,  цига", "со <b>вахна</b>,  цига"},
		{"перевод строки сохранён", "цӏа\nхи", "цӏа\nхиш", "цӏа\n<b>хиш</b>"},
	}
	for _, c := range cases {
		if got := markCorrections(c.typed, c.corrected); got != c.want {
			t.Errorf("%s: markCorrections(%q, %q) = %q, want %q", c.name, c.typed, c.corrected, got, c.want)
		}
	}

	// The card is sent as HTML now, so anything the checker hands back has to
	// survive it: an unescaped angle bracket would blank the whole message.
	if got := markCorrections("a < b", "a < b"); strings.Contains(got, " < ") {
		t.Errorf("angle bracket reached Telegram unescaped: %q", got)
	}
}

package net

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestQuizPromptFromMessage(t *testing.T) {
	cases := []struct {
		text, want string
	}{
		{"🧠 Викторина\n\nКак переводится на русский?\n\nдошам", "дошам"},
		{"🧠 Викторина\n\nКак сказать по-чеченски?\n\nкӀант ", "кӀант"},
		{"одна строка", "одна строка"},
		{"", ""},
	}
	for _, c := range cases {
		if got := quizPromptFromMessage(c.text); got != c.want {
			t.Errorf("quizPromptFromMessage(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// The keyboard that disables further answering is edited after the score is
// written, and callbacks run concurrently, so two taps inside that window both
// counted — a fast tapper could score every option on the card, the right one
// among them.
func TestClaimQuizAnswer_OneScorePerQuestion(t *testing.T) {
	n := &Net{}
	if !n.claimQuizAnswer(7, 42) {
		t.Fatal("the first answer was refused")
	}
	if n.claimQuizAnswer(7, 42) {
		t.Error("the same question scored twice")
	}
	if !n.claimQuizAnswer(7, 43) {
		t.Error("the next question was refused")
	}

	// Concurrent taps on one question: exactly one may write a score.
	var wg sync.WaitGroup
	var won atomic.Int32
	for range 32 {
		wg.Go(func() {
			if n.claimQuizAnswer(9, 1) {
				won.Add(1)
			}
		})
	}
	wg.Wait()
	if got := won.Load(); got != 1 {
		t.Errorf("%d of 32 concurrent taps scored, want exactly 1", got)
	}

	// The map is bounded, and a dropped claim only reopens the window on a
	// question nobody is looking at any more.
	for i := range maxQuizClaims + 10 {
		n.claimQuizAnswer(11, i)
	}
	if len(n.quizClaims) > maxQuizClaims {
		t.Errorf("claims grew to %d, past the %d cap", len(n.quizClaims), maxQuizClaims)
	}
}

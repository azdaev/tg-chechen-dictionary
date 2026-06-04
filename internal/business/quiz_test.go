package business

import (
	"chetoru/internal/models"
	"context"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestGenerateQuiz_PromptMatchesCorrectOption(t *testing.T) {
	// Keep the background pool refill away from the real API.
	stubDoshamAPI(t, http.StatusInternalServerError, ``)

	words := []models.RandomWord{
		{Chechen: "дитт", Russian: "дерево"},
		{Chechen: "цӏа", Russian: "дом"},
		{Chechen: "ӏаж", Russian: "яблоко"},
		{Chechen: "кхор", Russian: "груша"},
	}
	byChe := make(map[string]string)
	byRus := make(map[string]string)
	for _, w := range words {
		byChe[w.Chechen] = w.Russian
		byRus[w.Russian] = w.Chechen
	}

	b := &Business{log: logrus.New()}
	sawForward, sawReversed := false, false
	for range 30 {
		for _, w := range words {
			b.pool.insert(w)
		}
		q, err := b.GenerateQuiz(context.Background())
		if err != nil {
			t.Fatalf("GenerateQuiz: %v", err)
		}
		if len(q.Options) != quizOptionCount {
			t.Fatalf("options = %v, want %d", q.Options, quizOptionCount)
		}

		correct := q.Options[q.CorrectIdx]
		if q.Reversed {
			sawReversed = true
			if byRus[q.Prompt] != correct {
				t.Fatalf("reversed: prompt %q has correct option %q", q.Prompt, correct)
			}
		} else {
			sawForward = true
			if byChe[q.Prompt] != correct {
				t.Fatalf("forward: prompt %q has correct option %q", q.Prompt, correct)
			}
		}
	}
	if !sawForward || !sawReversed {
		t.Fatalf("both directions should appear in 30 runs (forward=%v reversed=%v)", sawForward, sawReversed)
	}
}

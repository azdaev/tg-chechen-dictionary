package net

import (
	"chetoru/internal/models"
	"testing"
)

func TestScorerName(t *testing.T) {
	cases := []struct {
		s    models.QuizScorer
		want string
	}{
		{models.QuizScorer{Username: "alice", FirstName: "Alice"}, "alice"},
		{models.QuizScorer{FirstName: "Боб"}, "Боб"},
		{models.QuizScorer{}, "Аноним"},
	}
	for _, c := range cases {
		if got := scorerName(c.s); got != c.want {
			t.Errorf("scorerName(%+v) = %q, want %q", c.s, got, c.want)
		}
	}
}

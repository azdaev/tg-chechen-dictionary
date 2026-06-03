package net

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestFormatGrammarCard(t *testing.T) {
	t.Run("noun with forms", func(t *testing.T) {
		g := &models.WordGrammar{
			Headword: "дог",
			POS:      "существительное",
			Forms:    []string{"деган", "дагна", "дегнаш"},
		}
		got := formatGrammarCard(g)
		want := "📖 <b>дог</b> · существительное\nФормы: деган, дагна, дегнаш"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("forms without confident POS", func(t *testing.T) {
		g := &models.WordGrammar{Headword: "къайлаха", Forms: []string{"къайлахо"}}
		got := formatGrammarCard(g)
		if !strings.HasPrefix(got, "📖 <b>къайлаха</b>") || strings.Contains(got, "·") {
			t.Errorf("unexpected card without POS: %q", got)
		}
		if !strings.Contains(got, "Формы: къайлахо") {
			t.Errorf("missing forms line: %q", got)
		}
	})

	t.Run("bare headword is suppressed", func(t *testing.T) {
		g := &models.WordGrammar{Headword: "дог"}
		if got := formatGrammarCard(g); got != "" {
			t.Errorf("expected empty card for bare headword, got %q", got)
		}
	})

	t.Run("long paradigm is truncated", func(t *testing.T) {
		forms := make([]string, maxGrammarForms+5)
		for i := range forms {
			forms[i] = "ф"
		}
		g := &models.WordGrammar{Headword: "x", POS: "существительное", Forms: forms}
		got := formatGrammarCard(g)
		if !strings.Contains(got, "… (+5)") {
			t.Errorf("expected overflow marker, got %q", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if got := formatGrammarCard(nil); got != "" {
			t.Errorf("expected empty for nil, got %q", got)
		}
	})
}

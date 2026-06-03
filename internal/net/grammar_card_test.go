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

	t.Run("idioms section", func(t *testing.T) {
		g := &models.WordGrammar{
			Headword: "дог",
			POS:      "существительное",
			Idioms: []models.Idiom{
				{Chechen: "дог тедан", Russian: "успокоить"},
				{Chechen: "дог эца", Russian: "утешить"},
			},
		}
		got := formatGrammarCard(g)
		if !strings.Contains(got, "💬 <b>Выражения:</b>") {
			t.Errorf("missing idioms header: %q", got)
		}
		if !strings.Contains(got, "• дог тедан — успокоить") || !strings.Contains(got, "• дог эца — утешить") {
			t.Errorf("missing idiom lines: %q", got)
		}
	})

	t.Run("idioms alone (no POS/forms) still shows", func(t *testing.T) {
		g := &models.WordGrammar{
			Headword: "x",
			Idioms:   []models.Idiom{{Chechen: "a", Russian: "b"}},
		}
		if got := formatGrammarCard(g); !strings.Contains(got, "• a — b") {
			t.Errorf("expected idiom-only card to render, got %q", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if got := formatGrammarCard(nil); got != "" {
			t.Errorf("expected empty for nil, got %q", got)
		}
	})
}

func TestGrammarSummaryLine(t *testing.T) {
	t.Run("pos and forms", func(t *testing.T) {
		g := &models.WordGrammar{
			POS:   "существительное",
			Forms: []string{"деган", "дагна"},
		}
		want := "<i>существительное · формы: деган, дагна</i>"
		if got := grammarSummaryLine(g); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("pos only", func(t *testing.T) {
		g := &models.WordGrammar{POS: "глагол"}
		if got := grammarSummaryLine(g); got != "<i>глагол</i>" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("forms truncated to cap", func(t *testing.T) {
		forms := make([]string, maxSummaryForms+3)
		for i := range forms {
			forms[i] = "ж"
		}
		g := &models.WordGrammar{Forms: forms}
		got := grammarSummaryLine(g)
		if want := maxSummaryForms; strings.Count(got, "ж") != want {
			t.Errorf("expected %d forms, got %q", want, got)
		}
	})

	t.Run("empty and nil give nothing", func(t *testing.T) {
		if got := grammarSummaryLine(nil); got != "" {
			t.Errorf("nil: got %q", got)
		}
		if got := grammarSummaryLine(&models.WordGrammar{Headword: "дог"}); got != "" {
			t.Errorf("headword-only: got %q", got)
		}
	})
}

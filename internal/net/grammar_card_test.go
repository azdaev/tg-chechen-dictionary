package net

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestFormatGrammarBlock(t *testing.T) {
	t.Run("noun with forms", func(t *testing.T) {
		g := &models.WordGrammar{
			Headword: "дог",
			POS:      "существительное",
			Forms:    []string{"деган", "дагна", "дегнаш"},
		}
		got := formatGrammarBlock(g, "")
		want := "🔤 <b>дог</b> · существительное\nФормы: деган, дагна, дегнаш"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("forms without confident POS", func(t *testing.T) {
		g := &models.WordGrammar{Headword: "къайлаха", Forms: []string{"къайлахо"}}
		got := formatGrammarBlock(g, "")
		if !strings.HasPrefix(got, "🔤 <b>къайлаха</b>") || strings.Contains(got, "·") {
			t.Errorf("unexpected card without POS: %q", got)
		}
		if !strings.Contains(got, "Формы: къайлахо") {
			t.Errorf("missing forms line: %q", got)
		}
	})

	t.Run("bare headword is suppressed", func(t *testing.T) {
		g := &models.WordGrammar{Headword: "дог"}
		if got := formatGrammarBlock(g, ""); got != "" {
			t.Errorf("expected empty card for bare headword, got %q", got)
		}
	})

	t.Run("long paradigm is truncated", func(t *testing.T) {
		forms := make([]string, maxGrammarForms+5)
		for i := range forms {
			forms[i] = "ф"
		}
		g := &models.WordGrammar{Headword: "x", POS: "существительное", Forms: forms}
		got := formatGrammarBlock(g, "")
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
		got := formatGrammarBlock(g, "")
		if !strings.Contains(got, "💬 <b>Выражения:</b>") {
			t.Errorf("missing idioms header: %q", got)
		}
		if !strings.Contains(got, "• <i>дог тедан → успокоить</i>") || !strings.Contains(got, "• <i>дог эца → утешить</i>") {
			t.Errorf("missing idiom lines: %q", got)
		}
	})

	t.Run("idioms alone (no POS/forms) still shows", func(t *testing.T) {
		g := &models.WordGrammar{
			Headword: "x",
			Idioms:   []models.Idiom{{Chechen: "a", Russian: "b"}},
		}
		if got := formatGrammarBlock(g, ""); !strings.Contains(got, "• <i>a → b</i>") {
			t.Errorf("expected idiom-only card to render, got %q", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		if got := formatGrammarBlock(nil, ""); got != "" {
			t.Errorf("expected empty for nil, got %q", got)
		}
	})
}

// Appended to a card, the block must not repeat what the card already says.
// «телефон» used to answer with three examples and then send a second message
// listing the same three under «Выражения».
func TestFormatGrammarBlock_MergedIntoTheCard(t *testing.T) {
	card := "телефон · <i>сущ.</i>\n<b>телефон</b>\n\n<i>телефон етта → звонить по телефону</i>"
	g := &models.WordGrammar{
		Headword: "телефон",
		POS:      "существительное",
		Forms:    []string{"телефонан", "телефонаш"},
		Idioms: []models.Idiom{
			{Chechen: "телефон етта", Russian: "звонить по телефону"},
			{Chechen: "телефон яло", Russian: "провести телефон"},
		},
	}
	got := formatGrammarBlock(g, card)

	if strings.Contains(got, "🔤") {
		t.Errorf("header repeated a word the card already names:\n%s", got)
	}
	if strings.Count(got, "телефон етта") != 0 {
		t.Errorf("idiom already shown on the card was repeated:\n%s", got)
	}
	if !strings.Contains(got, "телефон яло") {
		t.Errorf("a new idiom was dropped:\n%s", got)
	}
	if !strings.Contains(got, "Формы: телефонан, телефонаш") {
		t.Errorf("the paradigm is the whole point and it is missing:\n%s", got)
	}

	// Nothing new to add means no edit at all.
	covered := &models.WordGrammar{
		Headword: "телефон",
		POS:      "существительное",
		Idioms:   []models.Idiom{{Chechen: "телефон етта", Russian: "звонить по телефону"}},
	}
	if got := formatGrammarBlock(covered, card); got != "" {
		t.Errorf("block rendered nothing the card lacked: %q", got)
	}

	// A grammar entry for a different word keeps its header, or the forms would
	// look like they belong to the card's headword.
	other := &models.WordGrammar{Headword: "тилпу", POS: "существительное", Forms: []string{"тилпуш"}}
	if got := formatGrammarBlock(other, card); !strings.Contains(got, "🔤 <b>тилпу</b>") {
		t.Errorf("forms for another word lost their header:\n%s", got)
	}

	// A Russian lookup gets the paradigm of its Chechen gloss, so the word is on
	// the card but is not what heads it. Unlabelled, «Формы: къоламо̃…» under
	// «карандаш» reads as the forms of «карандаш».
	rusCard := "карандаш · <i>рус. → чеч., сущ.</i>\n<b>къолам</b>"
	gloss := &models.WordGrammar{Headword: "къолам", POS: "существительное", Forms: []string{"къоламан"}}
	if got := formatGrammarBlock(gloss, rusCard); !strings.Contains(got, "🔤 <b>къолам</b>") {
		t.Errorf("the paradigm of a gloss did not say which word it belongs to:\n%s", got)
	}
}

func TestGrammarSummaryLine(t *testing.T) {
	t.Run("pos and forms", func(t *testing.T) {
		g := &models.WordGrammar{
			POS:   "существительное",
			Forms: []string{"деган", "дагна"},
		}
		want := "существительное · формы: деган, дагна"
		if got := grammarSummaryLine(g); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("pos only", func(t *testing.T) {
		g := &models.WordGrammar{POS: "глагол"}
		if got := grammarSummaryLine(g); got != "глагол" {
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

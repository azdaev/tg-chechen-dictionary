package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"strings"
	"testing"
)

func TestInlineCardRendering(t *testing.T) {
	// The inline-sent message must match the text path's card, not the raw
	// gloss; the picker description is built from the data behind it.
	p := models.TranslationPairs{
		Original:  "Дом",
		Translate: "м 1) цӏа; деревянный ~- дечиган цӏа",
	}
	formatted := tools.FormatPairs([]models.TranslationPairs{p})
	if !strings.HasPrefix(formatted, "<b>Дом</b> — цӏа") {
		t.Errorf("formatted card = %q, want it to start with %q", formatted, "<b>Дом</b> — цӏа")
	}
	if strings.Contains(formatted, "~") || strings.Contains(formatted, "1)") {
		t.Errorf("raw gloss markup leaked into inline card:\n%s", formatted)
	}

	// Telegram renders the description as plain text, so a tag reaching it
	// shows up literally — which is what slicing the description out of the
	// rendered card started doing the moment that card grew a <b>.
	desc := inlineDescription(p.Translate)
	if strings.ContainsAny(desc, "<>") {
		t.Errorf("description carries markup: %q", desc)
	}
	if strings.Contains(desc, "\n") {
		t.Errorf("description not condensed to one line: %q", desc)
	}
	if !strings.Contains(desc, "цӏа") {
		t.Errorf("description lost the translation: %q", desc)
	}
}

func TestInlineDescriptionTruncates(t *testing.T) {
	desc := inlineDescription(strings.Repeat("слово ", 40))
	if n := len([]rune(desc)); n > inlineDescriptionRunes+1 { // +1 for the ellipsis
		t.Errorf("description is %d runes, want at most %d", n, inlineDescriptionRunes+1)
	}
	if !strings.HasSuffix(desc, "…") {
		t.Errorf("truncated description missing its ellipsis: %q", desc)
	}
	// The cut lands on a word boundary, so no half-word before the ellipsis.
	if strings.HasSuffix(desc, "сло…") {
		t.Errorf("description cut mid-word: %q", desc)
	}
}

// An outage used to answer nothing at all, leaving a dead picker with no
// explanation. Saying so is only safe if Telegram's edge does not keep the
// message after the outage ends — which is what CacheTime 0 and IsPersonal buy.
func TestInlineUnavailableIsNotEdgeCached(t *testing.T) {
	conf := inlineUnavailableConfig("q1")
	if conf.CacheTime != 0 {
		t.Errorf("CacheTime = %d, want 0 — an outage must not outlive itself", conf.CacheTime)
	}
	if !conf.IsPersonal {
		t.Error("IsPersonal = false; a shared cache entry would show the outage to everyone")
	}
	if len(conf.Results) != 1 {
		t.Fatalf("got %d results, want one explaining the failure", len(conf.Results))
	}
}

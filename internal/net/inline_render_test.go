package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"strings"
	"testing"
)

func TestInlineCardRendering(t *testing.T) {
	// The inline-sent message must match the text path's card, not the raw
	// gloss; the picker description is the card minus its headword.
	p := models.TranslationPairs{
		Original:  "Дом",
		Translate: "м 1) цӏа; деревянный ~- дечиган цӏа",
	}
	formatted := tools.FormatPairs([]models.TranslationPairs{p})
	if !strings.HasPrefix(formatted, "Дом — цӏа") {
		t.Errorf("formatted card = %q, want it to start with %q", formatted, "Дом — цӏа")
	}
	if strings.Contains(formatted, "~") || strings.Contains(formatted, "1)") {
		t.Errorf("raw gloss markup leaked into inline card:\n%s", formatted)
	}

	desc := strings.ReplaceAll(strings.TrimPrefix(formatted, tools.Clean(p.Original)+" — "), "\n", " · ")
	if strings.HasPrefix(desc, "Дом") {
		t.Errorf("description still carries the headword: %q", desc)
	}
	if !strings.Contains(desc, " · ") {
		t.Errorf("description not condensed to one line: %q", desc)
	}
}

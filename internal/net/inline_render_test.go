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
	if !strings.HasPrefix(formatted, "📘 <b>Дом</b>") || !strings.Contains(formatted, "• цӏа") {
		t.Errorf("formatted card = %q, want structured card with translation цӏа", formatted)
	}
	if strings.Contains(formatted, "~") || strings.Contains(formatted, "1)") {
		t.Errorf("raw gloss markup leaked into inline card:\n%s", formatted)
	}

	desc := inlineDescription(formatted, p.Original)
	if strings.HasPrefix(desc, "Дом") {
		t.Errorf("description still carries the headword: %q", desc)
	}
	if !strings.Contains(desc, " · ") {
		t.Errorf("description not condensed to one line: %q", desc)
	}
}

func TestInlineDescriptionWithLanguageLabels(t *testing.T) {
	formatted := tools.FormatPairs([]models.TranslationPairs{{
		Original: "Ряд", Translate: "1) могӏа; два ~а - шина могӏаре", OriginalLang: "RUS", TranslateLang: "CHE",
	}})

	desc := inlineDescription(formatted, "Ряд")
	if strings.Contains(desc, "Русский:") || strings.Contains(desc, "Чеченский:") {
		t.Errorf("language labels leaked into inline description: %q", desc)
	}
	if !strings.HasPrefix(desc, "могӏа") {
		t.Errorf("description should start with translated side, got %q", desc)
	}
	if !strings.Contains(desc, "1 прим.") {
		t.Errorf("description missing example count: %q", desc)
	}
}

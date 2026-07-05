package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestFormatPairs(t *testing.T) {
	t.Run("plain pair", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{Original: "дог", Translate: "сердце"}})
		want := "📘 <b>дог</b>\n\n<b>Перевод</b>\n• сердце"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("plain pair with language labels", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{
			Original: "Ряд", Translate: "могӏа", OriginalLang: "RUS", TranslateLang: "CHE",
		}})
		want := "📘 <b>Ряд</b>\n<i>с русского на чеченский</i>\n\n<b>Перевод</b>\n• могӏа"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("stored AI formatting wins", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{
			Original: "x", Translate: "y", FormattedChosen: "ai", FormattedAI: "<b>дог</b> — сердце",
		}})
		if got != "<b>дог</b> — сердце" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("numbered senses use the dictionary formatter", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{Original: "дита", Translate: "1) оставить 2) развестись"}})
		// FormatTranslationLite renders the headword + bulleted senses; just
		// assert it didn't fall through to the plain "a — b" form.
		if strings.Contains(got, "дита — 1)") {
			t.Errorf("complex entry fell through to plain formatting: %q", got)
		}
		if !strings.Contains(got, "дита") {
			t.Errorf("headword missing: %q", got)
		}
	})

	t.Run("dictionary formatter with language labels", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{
			Original: "Ряд", Translate: "1) могӏа; два ~а - шина могӏаре", OriginalLang: "RUS", TranslateLang: "CHE",
		}})
		if !strings.HasPrefix(got, "📘 <b>Ряд</b>\n<i>с русского на чеченский</i>\n\n<b>Перевод</b>\n• могӏа") {
			t.Errorf("structured header missing: %q", got)
		}
		if !strings.Contains(got, "два ряда → шина могӏаре") {
			t.Errorf("example missing after structured header: %q", got)
		}
	})

	t.Run("result is trimmed and pairs are separated", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{
			{Original: "дог", Translate: "сердце"},
			{Original: "куьг", Translate: "рука"},
		})
		if strings.HasSuffix(got, "\n") || strings.HasPrefix(got, "\n") {
			t.Errorf("not trimmed: %q", got)
		}
		if !strings.Contains(got, "📘 <b>дог</b>") || !strings.Contains(got, "📘 <b>куьг</b>") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if got := FormatPairs(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestHasDictionaryMarkup(t *testing.T) {
	for _, s := range []string{"1) оставить", "цӏа 2) дом", "~ие горы"} {
		if !hasDictionaryMarkup(s) {
			t.Errorf("hasDictionaryMarkup(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"сердце", "дом", "простой перевод"} {
		if hasDictionaryMarkup(s) {
			t.Errorf("hasDictionaryMarkup(%q) = true, want false", s)
		}
	}
}

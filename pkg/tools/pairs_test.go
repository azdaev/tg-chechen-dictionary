package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestFormatPairs(t *testing.T) {
	t.Run("plain pair with no language bolds the headword", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{Original: "дог", Translate: "сердце"}})
		if got != "<b>дог</b> — сердце" {
			t.Errorf("got %q, want %q", got, "<b>дог</b> — сердце")
		}
	})

	t.Run("bold follows the chechen side", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{{
			Original: "дог", OriginalLang: "CHE", Translate: "сердце", TranslateLang: "RUS",
		}})
		if got != "<b>дог</b> — сердце" {
			t.Errorf("chechen headword: got %q", got)
		}

		// Same pair from the other direction: the studied language is now the
		// gloss, so the bold moves with it instead of staying on the headword.
		got = FormatPairs([]models.TranslationPairs{{
			Original: "сердце", OriginalLang: "RUS", Translate: "дог", TranslateLang: "CHE",
		}})
		if got != "сердце — <b>дог</b>" {
			t.Errorf("russian headword: got %q", got)
		}
	})

	t.Run("neighbours sharing a headword merge into one numbered block", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{
			{Original: "Дитт", OriginalLang: "CHE", Translate: "дерево", TranslateLang: "RUS"},
			{Original: "дитт", OriginalLang: "CHE", Translate: "древо", TranslateLang: "RUS"},
			{Original: "Дитт", OriginalLang: "CHE", Translate: "ясень", TranslateLang: "RUS"},
		})
		want := "<b>Дитт</b>\n1. дерево\n2. древо\n3. ясень"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("an AI rendering never merges", func(t *testing.T) {
		// It brings its own headword line; folding it into a numbered list would
		// mean re-parsing a rendering a moderator already approved.
		got := FormatPairs([]models.TranslationPairs{
			{Original: "Дитт", OriginalLang: "CHE", Translate: "дерево", TranslateLang: "RUS"},
			{Original: "Дитт", OriginalLang: "CHE", Translate: "древо", TranslateLang: "RUS",
				FormattedChosen: "ai", FormattedAI: "<b>Дитт</b> — древо"},
		})
		want := "<b>Дитт</b> — дерево\n\n<b>Дитт</b> — древо"
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

	t.Run("result is trimmed and pairs are separated", func(t *testing.T) {
		got := FormatPairs([]models.TranslationPairs{
			{Original: "дог", Translate: "сердце"},
			{Original: "куьг", Translate: "рука"},
		})
		if strings.HasSuffix(got, "\n") || strings.HasPrefix(got, "\n") {
			t.Errorf("not trimmed: %q", got)
		}
		if got != "<b>дог</b> — сердце\n\n<b>куьг</b> — рука" {
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

package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestFormatPairs_BoldMarksChechen(t *testing.T) {
	got := FormatPairs([]models.TranslationPairs{
		{Original: "Карандаш", Translate: "къолам", OriginalLang: "RUS", TranslateLang: "CHE"},
		{Original: "дог", Translate: "сердце", OriginalLang: "CHE", TranslateLang: "RUS"},
		{Original: "дог", Translate: "сердце"},
	})
	want := "Карандаш — <b>къолам</b>\n<b>дог</b> — сердце\n<b>дог</b> — сердце"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatPairs_SkipsEmptySides(t *testing.T) {
	if got := FormatPairs([]models.TranslationPairs{
		{Original: "  ", Translate: "къолам"},
		{Original: "дом", Translate: ""},
	}); got != "" {
		t.Fatalf("expected no lines, got %q", got)
	}
}

// One block per headword: the same word from two source dictionaries is one
// card, not two, and the senses keep the order the dictionary gave them.
func TestFormatCard_OneBlockPerHeadword(t *testing.T) {
	card := FormatCard("куьг", []models.TranslationPairs{
		{Original: "куьг", Translate: "рука́ (кисть)", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", Subtype: 2, EntryIndex: 1},
		{Original: "куьг", Translate: "по́дпись", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", Subtype: 2, EntryIndex: 1},
		// The compact dictionary records no homonym number; index 0 must fold
		// into 1 rather than open a second block.
		{Original: "Куьг", Translate: "кисть", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD", EntryIndex: 0},
	})
	if strings.Count(card, "<b>куьг</b>") != 1 {
		t.Fatalf("expected exactly one headword line:\n%s", card)
	}
	if !strings.Contains(card, "1. рука́ (кисть)") || !strings.Contains(card, "2. по́дпись") {
		t.Fatalf("senses lost their source order:\n%s", card)
	}
	if !strings.Contains(card, "сущ.") {
		t.Fatalf("part of speech missing:\n%s", card)
	}
}

// цӀа the noun and цӀа the adverb are different words.
func TestFormatCard_HomonymsStaySeparate(t *testing.T) {
	card := FormatCard("цӀа", []models.TranslationPairs{
		{Original: "цӀа", Translate: "дом", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", Subtype: 2, EntryIndex: 1},
		{Original: "цӀа", Translate: "домо́й", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", Subtype: 3, EntryIndex: 2},
	})
	if !strings.Contains(card, "²") {
		t.Fatalf("second homonym not marked:\n%s", card)
	}
	if strings.Contains(card, "1. дом") {
		t.Fatalf("homonyms merged into one list:\n%s", card)
	}
}

// dosham's find is a substring search, so articles that merely mention the query
// come back too. «Нет» must not become a meaning of «къолам».
func TestFormatCard_MentionOnlyArticleIsNotAMeaning(t *testing.T) {
	card := FormatCard("къолам", []models.TranslationPairs{
		{Original: "къолам", Translate: "карандаш", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", EntryIndex: 1},
		{Original: "Нет", Translate: "хӏан-хӏа; къолам бац хьоьгахь? - нет ли у тебя карандаша?", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if strings.Contains(card, "хӏан-хӏа") {
		t.Fatalf("a mention became a meaning:\n%s", card)
	}
	if !strings.Contains(card, "нет ли у тебя карандаша") {
		t.Fatalf("its example for the word was dropped:\n%s", card)
	}
}

// A neighbouring headword is how dosham answers «дом» with «Домбра»: one footer
// line, never a sense.
func TestFormatCard_NeighboursGoToTheFooter(t *testing.T) {
	card := FormatCard("дом", []models.TranslationPairs{
		{Original: "дом", Translate: "цӀа", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 16, EntryType: "WORD"},
		{Original: "Домбра", Translate: "домбра", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(card, "рядом:") || !strings.Contains(card, "Домбра") {
		t.Fatalf("neighbour missing from footer:\n%s", card)
	}
	if strings.Contains(card, "1. домбра") {
		t.Fatalf("neighbour leaked into the senses:\n%s", card)
	}
}

// The Russian–Chechen article is the one corpus that still needs parsing: its
// glosses become senses and its tilde examples become example lines.
func TestFormatCard_ArticleBecomesSensesAndExamples(t *testing.T) {
	card := FormatCard("дом", []models.TranslationPairs{{
		Original:      "Дом",
		Translate:     "м 1) цӏа; деревянный ~- дечиган цӏа 2) (учреждение) цӏа; ~ отдыха - садаӏаран цӏа",
		OriginalLang:  "RUS",
		TranslateLang: "CHE",
		Rate:          100,
		EntryType:     "WORD",
	}})
	if strings.Contains(card, "м 1)") {
		t.Fatalf("the raw article reached the card:\n%s", card)
	}
	if !strings.Contains(card, "<b>цӏа</b>") {
		t.Fatalf("gloss missing:\n%s", card)
	}
	if !strings.Contains(card, "дечиган цӏа → деревянный дом") {
		t.Fatalf("tilde example missing or misordered:\n%s", card)
	}
	if !strings.Contains(card, "садаӏаран цӏа → дом отдыха") {
		t.Fatalf("second sense's example missing:\n%s", card)
	}
}

// Bold means Chechen and only Chechen, so a Russian qualifier trails the gloss.
func TestFormatCard_QualifiersLeaveTheBold(t *testing.T) {
	card := FormatCard("рука", []models.TranslationPairs{{
		Original: "Рука", Translate: "1) куьг 2) (почерк) хатӏ",
		OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD",
	}})
	if !strings.Contains(card, "<b>хатӏ</b> <i>(почерк)</i>") {
		t.Fatalf("qualifier still inside the bold:\n%s", card)
	}
}

package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

// «лоьма» matched «Лоьма-кӏорца» and «лоьманиг» by prefix and nothing else. The
// bot used to send that footer alone and count it as an answer, so the word
// never reached the missing-words report.
func TestCard_NeighboursAloneAreNotAnAnswer(t *testing.T) {
	r := Render("лоьма", []models.TranslationPairs{
		{Original: "Лоьма-кӏорца", Translate: "лем-Корц", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
		{Original: "лоьманиг", Translate: "львиный", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if r.Body != "" {
		t.Fatalf("neighbours rendered as a card body:\n%s", r.Body)
	}
	if len(r.Neighbours) != 2 {
		t.Fatalf("neighbours = %q, want both kept for the miss hint", r.Neighbours)
	}
	if FormatCard("лоьма", []models.TranslationPairs{
		{Original: "лоьманиг", Translate: "львиный", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	}) != "" {
		t.Error("FormatCard still returns a neighbours-only card")
	}
}

// «стрим» matched inside «гольфстрим» and produced no card at all. The handler
// used to paper over that with a raw pair dump.
func TestCard_BodyOnlyMentionIsNotAnAnswer(t *testing.T) {
	r := Render("стрим", []models.TranslationPairs{
		{Original: "Течение гольфстрим хи.", Translate: "гольфстрим", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "TEXT"},
	})
	if r.Body != "" || len(r.Neighbours) != 0 {
		t.Fatalf("a substring mention became an answer: body=%q neighbours=%q", r.Body, r.Neighbours)
	}
}

// A collocation asked for by name is an entry, not somebody's example. Once
// neighbours-only cards stopped counting as answers, this was the case that
// would have started answering «нет перевода» to a phrase the dictionary holds.
func TestCard_CollocationAskedByNameIsAnEntry(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Телефон болх беш яц", Translate: "Телефон не работает", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100, EntryType: "TEXT"},
	}
	body := Render("телефон болх беш яц", pairs).Body
	if body == "" {
		t.Fatal("a collocation the dictionary holds produced no card")
	}
	if !strings.Contains(body, "Телефон не работает") {
		t.Fatalf("the collocation lost its translation:\n%s", body)
	}

	// Asked for by a word inside it, the same row stays an example.
	card := FormatCard("телефон", append(pairs,
		models.TranslationPairs{Original: "телефон", Translate: "телефон", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"}))
	if !strings.Contains(card, "<i>Телефон болх беш яц → Телефон не работает</i>") {
		t.Fatalf("collocation stopped being an example of its own word:\n%s", card)
	}
}

// The academic corpus writes stress, the others do not. Both spellings reached
// the card as separate senses: "1. телефо́н 2. телефон".
func TestCard_StressVariantsAreOneSense(t *testing.T) {
	card := FormatCard("телефон", []models.TranslationPairs{
		{Original: "телефон", Translate: "телефо́н", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD"},
		{Original: "телефон", Translate: "телефон", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if strings.Contains(card, "1.") {
		t.Fatalf("one word rendered as two senses:\n%s", card)
	}
}

// The Russian–Chechen articles store capitalized headwords and the other three
// corpora do not, so «Карандаш» and «телефон» came out of one lookup shape.
func TestCard_HeadwordsAreLowercase(t *testing.T) {
	card := FormatCard("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.HasPrefix(card, "карандаш") {
		t.Fatalf("headword kept the source capitalization:\n%s", card)
	}
}

// Every layer that feeds this renderer matches on the folded key — the folded
// columns, the palochka cascade, rankPair's own folded bucket. The renderer
// matched only the strict key, so a word the bot had just found came back as
// «нет перевода». Each of these was verified dead before the fix.
func TestCard_FoldedHitStillRenders(t *testing.T) {
	cases := []struct {
		name, query, original string
	}{
		{"палочка опущена", "чегардиг", "Чӏегӏардиг"},
		{"ъ опущен", "колам", "къолам"},
		{"долгая гласная", "лесто", "лесто̃"},
		{"русское ударение", "рука", "ру́ка"},
	}
	for _, c := range cases {
		body := Render(c.query, []models.TranslationPairs{
			{Original: c.original, Translate: "перевод", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
		}).Body
		if body == "" {
			t.Errorf("%s: Card(%q) with stored %q rendered nothing", c.name, c.query, c.original)
		}
	}

	// Folding must not swallow the exact match into its block. Order here is
	// rankAndDedup's job, not the card's — pairs arrive ranked, exact first —
	// so the contract checked is that both keep their own spelling as a head
	// and the exact one still leads the card it was ranked to lead.
	body := Render("лом", []models.TranslationPairs{
		{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
		{Original: "ло̃м", Translate: "не тот", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	}).Body
	if !strings.HasPrefix(body, "<b>лом</b>") {
		t.Errorf("exact match lost the lead:\n%s", body)
	}
	if !strings.Contains(body, "ло̃м") {
		t.Errorf("folded homograph was absorbed into the exact block:\n%s", body)
	}
}

// A card can render without ever saying what the query means: dosham answers an
// inflected Russian form with the sentences that contain it and no entry of its
// own. Glossed is what lets the layer above tell that apart from an answer.
func TestRender_ExamplesWithoutAGlossAreNotGlossed(t *testing.T) {
	examples := []models.TranslationPairs{
		{Original: "жӏаьла караӏамо", Translate: "выдрессировать собаку", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 100},
		{Original: "жӏаьла дардан", Translate: "злить собаку", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 100},
	}
	r := Render("собаку", examples)
	if r.Body == "" {
		t.Fatal("the illustrations are worth showing when they are all there is")
	}
	if r.Glossed {
		t.Errorf("a card with no translation on it reports itself as an answer:\n%s", r.Body)
	}

	// An entry of its own is glossed, examples or not.
	if !Render("собака", append(examples, models.TranslationPairs{
		Original: "Собака", Translate: "ж жӏаьла", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100,
	})).Glossed {
		t.Error("a card that names the translation does not report it")
	}
}

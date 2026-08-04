package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

// Bold was the only thing marking the Chechen side, and the card never said so.
// Asked for a loanword the bot answered «телефон → 1. телефон» and there was no
// way to tell which of the two was the Chechen one.
func TestCard_EveryBlockNamesItsDirection(t *testing.T) {
	che, _ := Card("къолам", []models.TranslationPairs{
		{Original: "къолам", Translate: "карандаш", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if !strings.Contains(che, "чеч. → рус.") {
		t.Errorf("Chechen lookup did not say which side is Chechen:\n%s", che)
	}

	rus, _ := Card("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "м къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(rus, "рус. → чеч.") {
		t.Errorf("Russian lookup did not say which side is Chechen:\n%s", rus)
	}
}

// The corpora disagree about capitalizing glosses, so «куьг» answered «1. Рука»
// and «вода» answered «Хи» while every neighbouring card was lowercase. The
// sentence-shaped translation of a collocation keeps its capital —
// TestCard_CollocationAskedByNameIsAnEntry guards that side.
func TestCard_WordGlossesAreLowercase(t *testing.T) {
	body, _ := Card("куьг", []models.TranslationPairs{
		{Original: "Куьг", Translate: "Рука", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100, EntryType: "TEXT"},
	})
	if !strings.Contains(body, "рука") || strings.Contains(body, "Рука") {
		t.Errorf("gloss kept the source capitalization:\n%s", body)
	}
}

// «лом» is a Russian crowbar and a Chechen lion. Both spellings keyed the same
// block, so the card listed «лев» among the Chechen translations of «лом» —
// telling the user that the Chechen for «лом» is «лев».
func TestCard_CrossLanguageHomographsDoNotMerge(t *testing.T) {
	body, _ := Card("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
		{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", EntryIndex: 1},
	})
	for _, para := range strings.Split(body, "\n\n") {
		if strings.Contains(para, "ваба") && strings.Contains(para, "лев") {
			t.Fatalf("both readings landed in one block:\n%s", body)
		}
	}
	if !strings.Contains(body, "чеч. → рус.") || !strings.Contains(body, "рус. → чеч.") {
		t.Errorf("the two readings are not labelled apart:\n%s", body)
	}
}

// Grammar lives only under the Chechen headword, and dosham's search is literal:
// find("карандаш") returns no analyzed entry at all, because the corpus holding
// «къолам» spells its Russian side «каранда́ш». Asking by the Russian query threw
// the paradigm away for every Russian lookup.
func TestChechenSide_NamesTheWordGrammarBelongsTo(t *testing.T) {
	rus := ChechenSide("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "м къолам; химический ~ - шекъа долун къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if rus != "къолам" {
		t.Errorf("ChechenSide(карандаш) = %q, want къолам", rus)
	}

	che := ChechenSide("куьг", []models.TranslationPairs{
		{Original: "куьг", Translate: "рука́ (кисть)", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD"},
	})
	if che != "куьг" {
		t.Errorf("ChechenSide(куьг) = %q, want the headword itself", che)
	}

	// One gloss, one word: «лом, ваба (орудие)» is two spellings and a label.
	multi := ChechenSide("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба (орудие)", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if multi != "лом" {
		t.Errorf("ChechenSide picked %q out of a multi-variant gloss, want лом", multi)
	}

	if ChechenSide("нетслова", nil) != "" {
		t.Error("a card with no blocks named a word anyway")
	}
}

// Homonyms share a headword, so dosham's examples arrive filed under neither:
// «цӀа» is both the adverb «домой» and the noun «комната», and every example
// landed on whichever block sorted first. The card then read «домой» and, right
// under it, «перемерить комнату» — the wrong word taught with a straight face.
func TestCard_ExampleGoesToTheSenseItIllustrates(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "цӏа", Translate: "домой, в свой дом", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 3, EntryIndex: 2, Rate: 10000},
		{Original: "цӏа", Translate: "комната", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, EntryIndex: 1, Rate: 10000},
		{Original: "цӏа духадуста", Translate: "перемерить комнату", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 10000},
		{Original: "цӏа кхиа", Translate: "успеть домой", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 10000},
	}

	card := FormatCard("цӏа", pairs)
	room, home := strings.Index(card, "комната"), strings.Index(card, "домой, в свой дом")
	if room < 0 || home < 0 {
		t.Fatalf("both homonyms should have a block:\n%s", card)
	}
	if i := strings.Index(card, "перемерить комнату"); i < room {
		t.Errorf("the room example sits under «%s»:\n%s", map[bool]string{true: "домой"}[i > home], card)
	}
	if i := strings.Index(card, "успеть домой"); i < home || i > room {
		t.Errorf("the adverb's own example left its block:\n%s", card)
	}
}

// dosham holds «Как дела?» with the question mark a user never types, and the
// card compared the two raw: the phrase came back from the API, matched nothing,
// and the bot answered «нет перевода» while holding the exact translation.
func TestCard_PhrasePunctuationIsNotIdentity(t *testing.T) {
	card := FormatCard("как дела", []models.TranslationPairs{
		{Original: "Муха ду гӏуллакхаш?", Translate: "Как дела?", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 100},
	})
	if !strings.Contains(card, "Муха ду гӏуллакхаш?") {
		t.Fatalf("the phrase dosham holds did not make a card:\n%s", card)
	}

	// The same greeting, glossed twice with and without the mark, is one entry.
	greeting := FormatCard("доброе утро", []models.TranslationPairs{
		{Original: "Ӏуьйре дика хуьлда!", Translate: "доброе утро", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 16},
		{Original: "Ӏуьйре дика хуьлда!", Translate: "доброе утро!", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 16},
	})
	if n := strings.Count(greeting, "Ӏуьйре дика хуьлда!"); n != 1 {
		t.Errorf("the greeting is printed %d times:\n%s", n, greeting)
	}
}

// «спокойной ночи» is glossed twice — inside the article «Ночь» and inside
// «Пожелать» — and is nobody's headword. Every example was filed under an entry
// the query did not name, so all of them were dropped and the bot answered «нет
// перевода» while holding the translation twice over.
func TestCard_PhraseLivesOnlyInsideAnotherEntry(t *testing.T) {
	card := FormatCard("спокойной ночи", []models.TranslationPairs{
		{Original: "Ночь", Translate: "ж буьйса; спокойной ночи! - буьйса декъала хуьлда!; полярная ~ - къилбаседера буьйса", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(card, "буьйса декъала хуьлда!") {
		t.Fatalf("the gloss dosham holds did not make a card:\n%s", card)
	}
	if !strings.Contains(card, "рус. → чеч.") {
		t.Errorf("the card does not say which way it reads:\n%s", card)
	}
	// The article's own unrelated examples stay out of it.
	if strings.Contains(card, "къилбаседера") {
		t.Errorf("«полярная ночь» came along for the ride:\n%s", card)
	}
}

// The compact corpus lists a plural ending after the word and glues the next
// sense to its number. «салам, -аш, 2маршалла» is two ways to say «привет», and
// the card offered the whole string — commas, dash and digit — as Chechen.
func TestCard_PackedSensesAreUnpacked(t *testing.T) {
	card := FormatCard("привет", []models.TranslationPairs{
		{Original: "салам, -аш, 2маршалла", Translate: "привет", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "TEXT", Rate: 16},
	})
	if strings.Contains(card, "-аш") || strings.Contains(card, "2маршалла") {
		t.Fatalf("the corpus's own shorthand reached the user:\n%s", card)
	}
	for _, want := range []string{"салам", "маршалла"} {
		if !strings.Contains(card, want) {
			t.Errorf("%q is missing from:\n%s", want, card)
		}
	}

	// A gloss that merely lists variants is one sense and stays whole.
	plain := FormatCard("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба (орудие)", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100},
	})
	if !strings.Contains(plain, "лом, ваба") {
		t.Errorf("a multi-variant gloss was split:\n%s", plain)
	}
}

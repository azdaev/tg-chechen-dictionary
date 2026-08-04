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
	che := Render("къолам", []models.TranslationPairs{
		{Original: "къолам", Translate: "карандаш", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	}).Body
	if !strings.Contains(che, "чеч. → рус.") {
		t.Errorf("Chechen lookup did not say which side is Chechen:\n%s", che)
	}

	rus := Render("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "м къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	}).Body
	if !strings.Contains(rus, "рус. → чеч.") {
		t.Errorf("Russian lookup did not say which side is Chechen:\n%s", rus)
	}
}

// The corpora disagree about capitalizing glosses, so «куьг» answered «1. Рука»
// and «вода» answered «Хи» while every neighbouring card was lowercase. The
// sentence-shaped translation of a collocation keeps its capital —
// TestCard_CollocationAskedByNameIsAnEntry guards that side.
func TestCard_WordGlossesAreLowercase(t *testing.T) {
	body := Render("куьг", []models.TranslationPairs{
		{Original: "Куьг", Translate: "Рука", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100, EntryType: "TEXT"},
	}).Body
	if !strings.Contains(body, "рука") || strings.Contains(body, "Рука") {
		t.Errorf("gloss kept the source capitalization:\n%s", body)
	}
}

// «лом» is a Russian crowbar and a Chechen lion. Both spellings keyed the same
// block, so the card listed «лев» among the Chechen translations of «лом» —
// telling the user that the Chechen for «лом» is «лев».
func TestCard_CrossLanguageHomographsDoNotMerge(t *testing.T) {
	body := Render("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
		{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", EntryIndex: 1},
	}).Body
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
	rus := Render("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "м къолам; химический ~ - шекъа долун къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	}).Chechen
	if rus != "къолам" {
		t.Errorf("Chechen side of карандаш = %q, want къолам", rus)
	}

	che := Render("куьг", []models.TranslationPairs{
		{Original: "куьг", Translate: "рука́ (кисть)", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD"},
	}).Chechen
	if che != "куьг" {
		t.Errorf("Chechen side of куьг = %q, want the headword itself", che)
	}

	// One gloss, one word: «лом, ваба (орудие)» is two spellings and a label.
	multi := Render("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба (орудие)", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	}).Chechen
	if multi != "лом" {
		t.Errorf("the card picked %q out of a multi-variant gloss, want лом", multi)
	}

	if Render("нетслова", nil).Chechen != "" {
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

// Bold on the headword says «this side is Chechen». «Шпиц» glosses «собака» as
// "(собака) кӏезалг" and «Такса» writes its second entry as "2 ж (собака)
// такса", so the card bolded a Russian qualifier — and a homonym number and a
// gender marker — as the Chechen to say aloud.
func TestCard_HeadwordCarriesOnlyTheChechen(t *testing.T) {
	card := FormatCard("собака", []models.TranslationPairs{
		{Original: "Шпиц", Translate: "м (собака) кӏезалг", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
		{Original: "Такса", Translate: "2 ж (собака) такса", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	for _, bad := range []string{"<b>(собака)", "<b>2 ", "<b>ж "} {
		if strings.Contains(card, bad) {
			t.Errorf("%q is bold as Chechen:\n%s", bad, card)
		}
	}
	for _, want := range []string{"<b>кӏезалг</b> <i>(собака)</i>", "<b>такса</b> <i>(собака)</i>"} {
		if !strings.Contains(card, want) {
			t.Errorf("missing %q in:\n%s", want, card)
		}
	}
}

// The notes field carries two different things and only one is grammar. The
// compact corpus writes «мн. -аш»; the encyclopedic one writes a definition in
// Chechen, and the chip printed those as labels — «дог · чеч. → рус., сущ., 4
// хара йолуш ду», a noun that "has four holes".
func TestCard_ChipTakesOnlyGrammarNotes(t *testing.T) {
	card := FormatCard("дог", []models.TranslationPairs{
		{Original: "дог", Translate: "сердце", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 100, Notes: "4 хара йолуш ду"},
	})
	if strings.Contains(card, "хара йолуш") {
		t.Errorf("an encyclopedic definition is worn as a grammar label:\n%s", card)
	}
	if !strings.Contains(card, "сущ.") {
		t.Errorf("the part of speech went with it:\n%s", card)
	}

	// The real note still reaches the chip, even when a definition arrives first.
	withPlural := FormatCard("глаз", []models.TranslationPairs{
		{Original: "БӏаьргI", Translate: "Глаз", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 100, Notes: "Сагаран вока"},
		{Original: "бӏаьрг", Translate: "глаз", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 16, Notes: "мн. -аш"},
	})
	if !strings.Contains(withPlural, "мн. -аш") {
		t.Errorf("the plural note was crowded out by the definition:\n%s", withPlural)
	}
}

// Three corpora folded the marks a keyboard cannot type and the fourth did not.
// The Russian→Chechen articles are the one corpus that packs its entry into a
// single string, and its own matching compared the query strictly — so the
// cascade would look «кӏеда» up for someone who typed «кеда», dosham would
// answer with the article, and the renderer would throw it away and report that
// the word does not exist.
func TestCard_ArticleGlossFoldsLikeEveryOtherCorpus(t *testing.T) {
	pencil := []models.TranslationPairs{{
		Original: "Карандаш", Translate: "м къолам; химический ~ - шекъа долун къолам",
		OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100,
	}}
	strict := Render("къолам", pencil).Body
	folded := Render("колам", pencil).Body
	if strict == "" {
		t.Fatal("the strict spelling does not render, so the test proves nothing")
	}
	if folded != strict {
		t.Errorf("folded spelling rendered differently:\n strict=%q\n folded=%q", strict, folded)
	}

	// The example under it has to survive the same fold, or the card answers
	// with a bare gloss for exactly the queries that most need an example.
	if !strings.Contains(folded, "шекъа долун къолам → химический карандаш") {
		t.Errorf("the example was filtered out by the strict key:\n%s", folded)
	}

	// The strict spelling still wins where the two disagree: «ца» is a word of
	// its own, and its own entry leads the card ahead of any folded match.
	own := Render("ца", []models.TranslationPairs{
		{Original: "ца", Translate: "не", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Rate: 16},
		{Original: "Дом", Translate: "м цӏа", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100},
	}).Body
	if !strings.HasPrefix(own, "<b>ца</b>") {
		t.Errorf("the folded match displaced the word actually typed:\n%s", own)
	}
}

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
		{Original: "Карандаш", Translate: "м къолам", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
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
		{Original: "Лом", Translate: "м лом, ваба", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
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
		{Original: "Карандаш", Translate: "м къолам; химический ~ - шекъа долун къолам", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
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
		{Original: "Лом", Translate: "м лом, ваба (орудие)", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
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
		{Original: "Ночь", Translate: "ж буьйса; спокойной ночи! - буьйса декъала хуьлда!; полярная ~ - къилбаседера буьйса", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
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
		{Original: "Лом", Translate: "м лом, ваба (орудие)", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100},
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
	pairs := []models.TranslationPairs{
		{Original: "Шпиц", Translate: "м (собака) кӏезалг", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
		{Original: "Такса", Translate: "2 ж (собака) такса", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
	}
	card := FormatCard("кӏезалг", pairs)
	for _, bad := range []string{"<b>(собака)", "<b>2 ", "<b>ж "} {
		if strings.Contains(card, bad) {
			t.Errorf("%q is bold as Chechen:\n%s", bad, card)
		}
	}
	if !strings.Contains(card, "<b>кӏезалг</b> <i>(собака)</i>") {
		t.Errorf("the Chechen headword lost its qualifier:\n%s", card)
	}
}

// The qualifier names the query, the gloss does not. Asked how to say «собака»,
// the card answered «жӏаьла» and then three more blocks — «кӏезалг», «такса»,
// «эр» — each headed by a Chechen word under a «чеч. → рус.» line, in a card
// the user opened by typing Russian. The article mentions the query while
// disambiguating a word of its own; that is not an answer to it.
func TestCard_QualifierIsNotTheGloss(t *testing.T) {
	card := FormatCard("собака", []models.TranslationPairs{
		{Original: "Собака", Translate: "ж жӏаьла", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
		{Original: "Шпиц", Translate: "м (собака) кӏезалг", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
		{Original: "Такса", Translate: "2 ж (собака) такса", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(card, "<b>жӏаьла</b>") {
		t.Fatalf("the answer to the query is missing:\n%s", card)
	}
	if strings.Contains(card, "чеч. → рус.") {
		t.Errorf("a Russian lookup produced a Chechen-headed block:\n%s", card)
	}
	if n := strings.Count(card, "→ чеч."); n != 1 {
		t.Errorf("the card has %d blocks, want the one the query asked for:\n%s", n, card)
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
		OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100,
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
		{Original: "Дом", Translate: "м цӏа", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100},
	}).Body
	if !strings.HasPrefix(own, "<b>ца</b>") {
		t.Errorf("the folded match displaced the word actually typed:\n%s", own)
	}
}

// The same headword must not appear twice on one card. dosham glosses «Касатка»
// as «ж (ласточка) чӏегӏардиг», so the card was handed the head «(ласточка)
// чӏегӏардиг» — keyed whole, that is a different word from «чӏегӏардиг», and the
// user read the word they had just looked up a second time, in brackets, with
// its own direction line under it.
func TestCard_QualifiedHeadIsTheSameWord(t *testing.T) {
	card := FormatCard("чӏегӏардиг", []models.TranslationPairs{
		{Original: "чӏегӏардиг", Translate: "ласточка", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD", EntryIndex: 1},
		{Original: "Касатка", Translate: "ж (ласточка) чӏегӏардиг", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
	})
	if n := strings.Count(card, "чӏегӏардиг</b>"); n != 1 {
		t.Errorf("the headword is printed %d times:\n%s", n, card)
	}
	for _, want := range []string{"1. ласточка", "2. касатка"} {
		if !strings.Contains(card, want) {
			t.Errorf("missing %q:\n%s", want, card)
		}
	}
	// And the qualifier goes with it: the card's own first line already says
	// «ласточка», so repeating it in brackets on the headword says nothing.
	if strings.Contains(card, "(ласточка)") {
		t.Errorf("the qualifier repeats a sense:\n%s", card)
	}

	// A qualifier that is not a sense still earns its place: «Шпиц — м (собака)
	// кӏезалг» is how the user learns which кӏезалг this is.
	dog := FormatCard("кӏезалг", []models.TranslationPairs{
		{Original: "Шпиц", Translate: "м (собака) кӏезалг", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(dog, "<b>кӏезалг</b> <i>(собака)</i>") {
		t.Errorf("a qualifier that disambiguates was dropped:\n%s", dog)
	}
}

// Chechen builds compounds freely, and dosham holds a lot of them. «нана» —
// mother — came back illustrated by three lines about the muscle that moves the
// thumb («Нана-пӏелг»), which took half the card's six example slots before the
// one sentence that uses the word itself.
func TestCard_CompoundExamplesYieldToRealOnes(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "нана", Translate: "мать", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD"},
	}
	for _, ex := range []struct{ che, rus string }{
		{"Нана-пӏелг дӏабуьгу еха оьзг", "Длинная мышца, отводящая большой палец кисти"},
		{"Нана-пӏелг сатто еха оьзг", "Длинный сгибатель большого пальца кисти"},
		{"Нана-маьӏда", "материнская порода"},
		{"Нана юй хьан?", "мать у тебя есть?"},
	} {
		pairs = append(pairs, models.TranslationPairs{
			Original: ex.che, Translate: ex.rus,
			OriginalLang: "CHE", TranslateLang: "RUS", Rate: 100, EntryType: "TEXT",
		})
	}

	card := FormatCard("нана", pairs)
	free := strings.Index(card, "Нана юй хьан?")
	compound := strings.Index(card, "Нана-пӏелг дӏабуьгу")
	if free < 0 || compound < 0 {
		t.Fatalf("both kinds of example should be on the card:\n%s", card)
	}
	if free > compound {
		t.Errorf("the compounds lead the card:\n%s", card)
	}

	// The dictionary's own order stands inside each group.
	if a, b := strings.Index(card, "Нана-пӏелг дӏабуьгу"), strings.Index(card, "Нана-пӏелг сатто"); a > b {
		t.Errorf("the source order of the compounds was shuffled:\n%s", card)
	}
}

// Which side is a packed article cannot be read off the languages. The local
// lookup leads with the side that matched the query, so a reverse hit arrives
// with its sides swapped — and «TranslateLang == CHE» then calls a compact pair
// an article and a real article a plain gloss.
func TestCard_ArticleIsToldApartFromAPlainPair(t *testing.T) {
	// The compact corpus, matched from the Russian side: one entry, two senses.
	compact := FormatCard("привет", []models.TranslationPairs{{
		Original: "привет", Translate: "салам, -аш, 2маршалла",
		OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "TEXT", Rate: 16, EntryIndex: 1,
	}})
	if !strings.HasPrefix(compact, "привет · <i>рус. → чеч.") {
		t.Errorf("a Russian query was answered as Chechen:\n%s", compact)
	}
	for _, want := range []string{"<b>салам</b>", "<b>маршалла</b>"} {
		if !strings.Contains(compact, want) {
			t.Errorf("missing %q — the packed-article parser ran on a plain pair:\n%s", want, compact)
		}
	}

	// The articles corpus, matched from the Chechen side: still an article.
	article := FormatCard("къолам", []models.TranslationPairs{{
		Original: "м къолам; химический ~ - шекъа долун къолам", Translate: "Карандаш",
		OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Rate: 100, Packed: true,
	}})
	if !strings.Contains(article, "<b>къолам</b>") || !strings.Contains(article, "карандаш") {
		t.Errorf("the article was not parsed, so the card is empty or raw:\n%s", article)
	}
	if !strings.Contains(article, "шекъа долун къолам → химический карандаш") {
		t.Errorf("the article's example was lost:\n%s", article)
	}
}

// The grammar note describes dosham's own headword, which is always the Chechen
// side. Under a Russian headword the senses are different Chechen words, and the
// note was taken from whichever pair happened to carry one: «дом» announced
// «сущ., мн. -аш» — the plural of «хӀусам» — directly above the line «1. цӏа».
func TestCard_GrammarBelongsToTheWordItLabels(t *testing.T) {
	card := FormatCard("дом", []models.TranslationPairs{
		{Original: "Дом", Translate: "м цӏа", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100},
		{Original: "хӀусам", Translate: "дом", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 16, Notes: "мн. -аш"},
	})
	if !strings.Contains(card, "1. <b>цӏа</b>") {
		t.Fatalf("the card does not lead with цӏа, so the test proves nothing:\n%s", card)
	}
	if strings.Contains(card, "мн. -аш") {
		t.Errorf("the second sense's plural is worn by the first:\n%s", card)
	}

	// The same note on the word the card does lead with still shows.
	own := FormatCard("хӀусам", []models.TranslationPairs{
		{Original: "хӀусам", Translate: "дом", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 16, Notes: "мн. -аш"},
	})
	if !strings.Contains(own, "мн. -аш") {
		t.Errorf("the headword's own plural was dropped:\n%s", own)
	}

	// And a Russian headword whose leading gloss carries the note keeps it.
	lead := FormatCard("вода", []models.TranslationPairs{
		{Original: "хи", Translate: "вода", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Subtype: 2, Rate: 16, Notes: "мн. -ш"},
	})
	if !strings.Contains(lead, "мн. -ш") {
		t.Errorf("the leading gloss's own plural was dropped:\n%s", lead)
	}
}

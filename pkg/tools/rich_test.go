package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
	"testing"
)

// che builds a Chechen-headword entry: dosham's own headword is the Chechen
// side, and the article is packed into one string.
func chePair(head, gloss string) models.TranslationPairs {
	return models.TranslationPairs{
		Original: head, Translate: gloss,
		OriginalLang: "CHE", TranslateLang: "RUS",
		Rate: 15, EntryType: "WORD", Subtype: 2, EntryIndex: 1,
	}
}

func chePhrase(che, rus string) models.TranslationPairs {
	return models.TranslationPairs{
		Original: che, Translate: rus,
		OriginalLang: "CHE", TranslateLang: "RUS",
		Rate: 100, EntryType: "TEXT", EntryIndex: 1,
	}
}

// The plain card says «который язык где» with bold and an arrow, and a learner
// cannot decode either without already knowing the answer. The rich card names
// both columns, so the Chechen side is identified by a word, every time.
func TestRichCard_NamesBothLanguages(t *testing.T) {
	card := FormatRichCard("къолам", []models.TranslationPairs{
		chePair("къолам", "карандаш"),
		chePhrase("къолам ирбан", "заострить карандаш"),
	})
	for _, want := range []string{
		"<h3>къолам</h3>",
		"<th>чеченский</th><th>русский</th>",
		"<td>къолам ирбан</td><td>заострить карандаш</td>",
		RichFooter,
	} {
		if !strings.Contains(card, want) {
			t.Errorf("rich card is missing %q:\n%s", want, card)
		}
	}
}

// Several senses become a real list rather than digits typed into the text.
func TestRichCard_SensesAreAList(t *testing.T) {
	card := FormatRichCard("къолам", []models.TranslationPairs{
		chePair("къолам", "карандаш"),
		chePair("къолам", "калам"),
	})
	if !strings.Contains(card, "<ol><li>карандаш</li><li>калам</li></ol>") {
		t.Errorf("senses did not become a list:\n%s", card)
	}
	// One sense is a sentence, not a list of one.
	single := FormatRichCard("къолам", []models.TranslationPairs{chePair("къолам", "карандаш")})
	if strings.Contains(single, "<ol>") {
		t.Errorf("a lone sense was rendered as a list:\n%s", single)
	}
}

// The whole point of the change: the plain card throws away every example past
// the sixth, and «собака» has twenty in the dictionary.
func TestRichCard_ExtraExamplesFold(t *testing.T) {
	pairs := []models.TranslationPairs{chePair("говр", "лошадь")}
	for i := range maxRichExampleRows + 3 {
		pairs = append(pairs, chePhrase(fmt.Sprintf("говр %d", i), fmt.Sprintf("лошадь %d", i)))
	}
	card := FormatRichCard("говр", pairs)
	if !strings.Contains(card, "<details><summary>Ещё 3 примера</summary>") {
		t.Errorf("the examples past the cap did not fold into <details>:\n%s", card)
	}
	if got, want := strings.Count(card, "<tr><td>"), maxRichExampleRows+3; got != want {
		t.Errorf("rows = %d, want %d — the rich card must not drop what the plain one caps", got, want)
	}

	// Nothing to fold, no disclosure widget to open.
	few := FormatRichCard("говр", []models.TranslationPairs{
		chePair("говр", "лошадь"), chePhrase("дика говр", "хорошая лошадь"),
	})
	if strings.Contains(few, "<details>") {
		t.Errorf("a card with two examples grew a <details>:\n%s", few)
	}
}

// A homonym number belongs outside the bold on the plain card and inside the
// heading on the rich one — but «¹» must never appear on a word that has no
// homonym, which is what superscript(1) returns.
func TestRichCard_HomonymNumber(t *testing.T) {
	plain := FormatCard("къолам", []models.TranslationPairs{chePair("къолам", "карандаш")})
	rich := FormatRichCard("къолам", []models.TranslationPairs{chePair("къолам", "карандаш")})
	for name, card := range map[string]string{"plain": plain, "rich": rich} {
		if strings.Contains(card, "¹") {
			t.Errorf("%s card numbered a word with no homonyms:\n%s", name, card)
		}
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{1: "пример", 2: "примера", 4: "примера", 5: "примеров",
		11: "примеров", 14: "примеров", 21: "пример", 22: "примера", 100: "примеров"}
	for n, want := range cases {
		if got := plural(n, "пример", "примера", "примеров"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}

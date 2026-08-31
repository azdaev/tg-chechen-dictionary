package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestParseArticle_SensesAndExamples(t *testing.T) {
	glosses, examples := ParseArticle("Дом", "м 1) цӏа; деревянный ~- дечиган цӏа 2) (учреждение) цӏа; ~ отдыха - садаӏаран цӏа")
	if len(glosses) != 2 || glosses[0] != "цӏа" || glosses[1] != "(учреждение) цӏа" {
		t.Fatalf("glosses = %q", glosses)
	}
	if len(examples) != 2 {
		t.Fatalf("examples = %+v", examples)
	}
	if examples[0].chechen != "дечиган цӏа" || examples[0].russian != "деревянный дом" {
		t.Fatalf("example sides wrong: %+v", examples[0])
	}
}

// The label heading a sense is metadata, not a meaning. «Идти» used to open on
// a sense that read only "несов.".
func TestParseArticle_StripsSenseLabels(t *testing.T) {
	glosses, _ := ParseArticle("Идти", "несов. 1) несов. (двигаться) даха 2) нареч. цӏа")
	for _, g := range glosses {
		if strings.HasPrefix(g, "несов") || strings.HasPrefix(g, "нареч") {
			t.Fatalf("label survived as a meaning: %q", g)
		}
	}
}

// The structured pass wins over the regex, and its note becomes the card's
// qualifier convention so render lifts it out of the bold.
func TestArticleParts_PrefersStructured(t *testing.T) {
	p := models.TranslationPairs{
		Structured: `{"senses":[{"gloss":"къолам ирбан","note":"заострить"}],
		              "examples":[{"ce":"къолам ирбан","ru":"заострить карандаш"}]}`,
	}
	glosses, examples := articleParts(p, "Заострить", "сов.: заострить карандаш - къолам ирбан")
	if len(glosses) != 1 || glosses[0] != "(заострить) къолам ирбан" {
		t.Fatalf("glosses = %q", glosses)
	}
	if len(examples) != 1 || examples[0].chechen != "къолам ирбан" {
		t.Fatalf("examples = %+v", examples)
	}
}

// Malformed or empty structure must not blank the card: the regex answer stands.
func TestArticleParts_FallsBackOnBadJSON(t *testing.T) {
	p := models.TranslationPairs{Structured: "{not json"}
	glosses, _ := articleParts(p, "Дом", "м 1) цӏа")
	if len(glosses) != 1 || glosses[0] != "цӏа" {
		t.Fatalf("fallback did not run: %q", glosses)
	}
}

// The Russian articles gloss with a list — «Новинка — ж керланиг, керла
// хӏума» — and the block was headed by the whole list, so every article that
// named the word made a separate Chechen entry out of it. «керланиг» came back
// as four entries and «сингаттаме» as thirteen, with the meanings the reader
// wanted spread across all of them.
func TestCard_SynonymListIsOneWord(t *testing.T) {
	article := func(head, gloss string) models.TranslationPairs {
		return models.TranslationPairs{Original: head, Translate: gloss,
			OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"}
	}
	body := Render("керланиг", []models.TranslationPairs{
		article("Новинка", "ж керланиг, керла хӏума"),
		article("Новость", "ж керланиг, керла хӏума"),
		article("Новичок", "м керланиг, керла дешархо"),
	}).Body
	if n := len(headLangs(body)); n != 1 {
		t.Fatalf("one word rendered as %d entries:\n%s", n, body)
	}
	for _, want := range []string{"новинка", "новость", "новичок"} {
		if !strings.Contains(body, want) {
			t.Errorf("meaning %q lost from the card:\n%s", want, body)
		}
	}

	// A phrase the word appears in is not a synonym of it, and keeps its own
	// heading: «дог эшна сингаттаме хилар» is not «сингаттаме».
	body = Render("сингаттаме", []models.TranslationPairs{
		article("Унылость", "ж дог эшна сингаттаме хилар"),
	}).Body
	if !strings.Contains(body, "дог эшна сингаттаме хилар") {
		t.Errorf("a phrase was cut down to the word inside it:\n%s", body)
	}
}

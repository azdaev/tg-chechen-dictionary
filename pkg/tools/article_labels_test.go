package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func article(head, body string) models.TranslationPairs {
	return models.TranslationPairs{
		Original: head, Translate: body,
		OriginalLang: "RUS", TranslateLang: "CHE",
		Rate: 100, EntryType: "WORD",
	}
}

// Live «Телефонировать». stripLabels has no pattern for "и без доп.", so the
// leftover metadata rode into the card as a bold headword.
func TestArticle_UnstrippedLabelNeverBecomesAHeadword(t *testing.T) {
	card := FormatCard("телефон", []models.TranslationPairs{
		article("Телефонировать", "сов. и несов., что, о чём и без доп. телефон тоха, телефон етта"),
	})
	if strings.Contains(card, "доп.") || strings.Contains(card, "чём") {
		t.Fatalf("grammar label reached the card:\n%s", card)
	}
	if !strings.Contains(card, "телефон тоха") {
		t.Fatalf("the translation behind the label was lost:\n%s", card)
	}
}

// Live «Трезвонит»: the whole first sense is an example, separator and all.
func TestArticle_DashClauseIsAnExampleNotAGloss(t *testing.T) {
	card := FormatCard("телефон", []models.TranslationPairs{
		article("Трезвонит", "телефон - ӏуьйранна дуьйна телефон ека; ~ в дверь — неӏ етта"),
	})
	if strings.Contains(card, "телефон - ") {
		t.Fatalf("an example was rendered as a headword:\n%s", card)
	}
}

// Live «Заострить»: "сов.: заострить карандаш - къолам ирбан" produced the
// headword «заострить карандаш - къолам ирбан» over the sense «заострить».
func TestArticle_ColonLabelledExampleStaysAnExample(t *testing.T) {
	card := FormatCard("карандаш", []models.TranslationPairs{
		article("Карандаш", "м къолам"),
		article("Заострить", "сов.: заострить карандаш - къолам ирбан"),
	})
	if strings.Contains(card, "заострить карандаш - ") {
		t.Fatalf("an example was rendered as a headword:\n%s", card)
	}
	if !strings.Contains(card, "къолам ирбан → заострить карандаш") {
		t.Fatalf("the example was dropped instead of rehomed:\n%s", card)
	}
}

// The ordinary article must keep working: one clause, no separator, real gloss.
func TestArticle_PlainGlossSurvives(t *testing.T) {
	card := FormatCard("дом", []models.TranslationPairs{
		article("Дом", "м 1) цӏа; деревянный ~- дечиган цӏа"),
	})
	if !strings.Contains(card, "<b>цӏа</b>") {
		t.Fatalf("plain gloss lost:\n%s", card)
	}
	if !strings.Contains(card, "дечиган цӏа → деревянный дом") {
		t.Fatalf("plain example lost:\n%s", card)
	}
}

// The LLM article parser hands back the odd sense with the source's own
// bookkeeping still on it: «Лев» arrived as "2 м лев (Болгарера ахча)" under a
// card that numbers its senses itself.
func TestArticle_StructuredGlossLosesItsSenseNumber(t *testing.T) {
	p := article("Лев", "ӏ м зоол. лом")
	p.Structured = `{"senses":[{"gloss":"цоькъа лом"},{"gloss":"2 м лев (Болгарера ахча)"}]}`
	card := FormatCard("лев", []models.TranslationPairs{p})

	if strings.Contains(card, "2 м лев") {
		t.Fatalf("sense number and gender reached the card:\n%s", card)
	}
	if !strings.Contains(card, "лев (Болгарера ахча)") {
		t.Fatalf("the sense itself was lost:\n%s", card)
	}
}

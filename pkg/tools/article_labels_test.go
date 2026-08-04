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

// «Спасибо» is one word — баркалла — and the card offered five senses:
// «частица баркалла», «, кому баркалла ду», «с баркалла» and two real ones.
// Cutting a gloss back to its last period leaves it starting mid-label, and
// every stripping pattern is anchored, so a leading comma stopped all of them.
func TestParseArticle_LabelRemnantsAreNotGlosses(t *testing.T) {
	glosses, _ := ParseArticle("спасибо",
		"ӏ. частица баркалла; ~ за внимание -ладогӏарна баркалла; "+
			"2. в знач. сказ., кому баркалла ду; большое вам ~! - доккха баркалла ду шуна!; "+
			"3. в знач. сущ. с баркалла; [он] и ~ не сказал - [цо] баркалла а ца элира")

	if len(glosses) == 0 {
		t.Fatal("the article lost every meaning")
	}
	for _, g := range glosses {
		if !strings.HasPrefix(g, "баркалла") {
			t.Errorf("gloss %q is not the word, it is what was left of a label", g)
		}
	}
}

// «Камень» opens "м, тж. собир. тӏулг". Abbreviations were expanded before the
// labels were stripped, so «собир.» became «(собирательное)» and took its period
// with it — nothing was left to cut at, and the first Chechen word the card
// offered for «камень» was the gender marker «м».
func TestParseArticle_LabelsAreStrippedBeforeTheirPeriodsAreSpent(t *testing.T) {
	glosses, _ := ParseArticle("Камень", "м, тж. собир. тӏулг; драгоценный ~- мехала тӏулг")
	if len(glosses) == 0 || glosses[0] != "тӏулг" {
		t.Fatalf("glosses = %q, want тӏулг alone", glosses)
	}

	// A period inside a qualifier is not a label's: cutting at the one in «род.»
	// left «матери) нана», a closing bracket with no opening one.
	mother, _ := ParseArticle("Мать", "ж (род. матери) нана")
	if len(mother) == 0 || strings.HasPrefix(mother[0], "матери)") {
		t.Errorf("glosses = %q, want the qualifier kept whole", mother)
	}

	// A cross-reference points at another entry; the word it points at is not
	// the translation. «Камешек» opens "м уменьш. от камень тӏулг".
	pebble, _ := ParseArticle("Камешек", "м уменьш. от камень тӏулг")
	if len(pebble) == 0 || pebble[0] != "тӏулг" {
		t.Errorf("glosses = %q, want тӏулг alone", pebble)
	}
}

// «Один» opens "м (одна ж, одно с; одни мн.) числ. цхьаъ" — the bracket lists
// the Russian headword's other forms, not what the word means. The card offered
// «(одна ж, одно с» as its first Chechen translation, bracket cut in half.
func TestParseArticle_FormListIsNotAMeaning(t *testing.T) {
	glosses, _ := ParseArticle("Один", "м (одна ж, одно с; одни мн.) ӏ. (т.к. ед.) числ. цхьаъ; ~ раз - цкъа")
	if len(glosses) == 0 || strings.Contains(glosses[0], "одна ж") {
		t.Fatalf("glosses = %q, want the form list gone", glosses)
	}

	// A bracket that says what the sense means is one phrase, and it stays.
	for _, keep := range []string{"(глава дома, семьи) да", "(орудие) лом", "(тот же самый) цхьана"} {
		if got := stripFormList(keep); got != keep {
			t.Errorf("stripFormList(%q) = %q, want it untouched", keep, got)
		}
	}
	// Two short items are a form list however the corpus punctuates them.
	for _, drop := range []string{"(двадцати, двадцатью) ткъа", "(одна ж, одно с; одни мн.) цхьаъ"} {
		if got := stripFormList(drop); strings.HasPrefix(got, "(") {
			t.Errorf("stripFormList(%q) = %q, want the list gone", drop, got)
		}
	}
}

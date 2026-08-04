package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

func TestCrossRef(t *testing.T) {
	ref := func(gloss string) models.TranslationPairs {
		return models.TranslationPairs{Original: "ваха", Translate: gloss,
			OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"}
	}
	cases := []struct {
		name  string
		query string
		pairs []models.TranslationPairs
		want  string
	}{
		{"одна отсылка", "ваха", []models.TranslationPairs{ref("см. даха")}, "даха"},
		{"обе статьи туда же", "ваха", []models.TranslationPairs{ref("см. даха"), ref("см. даха")}, "даха"},
		// Homonyms pointing at different words: answering with one of them
		// hides the other.
		{"расходятся", "ваха", []models.TranslationPairs{ref("см. даха"), ref("см. дахо")}, ""},
		{"это перевод, а не отсылка", "ваха", []models.TranslationPairs{ref("жить")}, ""},
		{"отсылка внутри перевода", "ваха", []models.TranslationPairs{ref("жить, см. даха")}, ""},
		// An example that merely contains the word says nothing about where the
		// word is defined.
		{"пример, а не статья", "ваха", []models.TranslationPairs{
			{Original: "ваха хӏусам чу", Translate: "см. даха", EntryType: "TEXT"}}, ""},
	}
	for _, c := range cases {
		if got := CrossRef(c.query, c.pairs); got != c.want {
			t.Errorf("%s: CrossRef = %q, want %q", c.name, got, c.want)
		}
	}

	// A second entry under the same headword that does translate the word
	// leaves nothing to follow.
	if got := CrossRef("ваха", []models.TranslationPairs{ref("см. даха"), ref("жить")}); got != "" {
		t.Errorf("CrossRef = %q; the word is translated here", got)
	}
}

// «яхчийта — понуд. от яхча» names two Chechen words and translates neither.
// Unlike «см.» the pointer is not an equivalence — a causative is not its base
// verb — so the base is reported for showing beside the entry, not instead.
func TestDerivedFrom(t *testing.T) {
	pair := func(head, gloss string) models.TranslationPairs {
		return models.TranslationPairs{Original: head, Translate: gloss,
			OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"}
	}
	cases := []struct{ name, query, gloss, want string }{
		{"понудительная", "яхчийта", "понуд. от яхча", "яхча"},
		{"потенциальная", "диттадала", "потенц. от дитта", "дитта"},
		{"без пробела", "юхаловзадала", "потенц.от юхаловза", "юхаловза"},
		{"прилагательное", "бежан", "прил. к бажа", "бажа"},
		{"с надстрочным номером", "вахавала", "потенц. от ваха¹,²", "ваха"},
		{"«см.» — не сюда", "ваха", "см. даха", ""},
		{"это перевод", "бежан", "табунный", ""},
	}
	for _, c := range cases {
		if got := DerivedFrom(c.query, []models.TranslationPairs{pair(c.query, c.gloss)}); got != c.want {
			t.Errorf("%s: DerivedFrom(%q) = %q, want %q", c.name, c.gloss, got, c.want)
		}
	}

	// A sense of its own leaves nothing to explain.
	if got := DerivedFrom("бежан", []models.TranslationPairs{
		pair("бежан", "прил. к бажа"), pair("бежан", "табунный"),
	}); got != "" {
		t.Errorf("DerivedFrom = %q; the word is translated here", got)
	}
}

// The academic corpus opens a sense with its labels — «прям., перен.
// закали́ться» — and the base-word line used to quote «прям.» as the meaning.
func TestFirstGloss_SkipsLabels(t *testing.T) {
	cases := []struct{ gloss, want string }{
		{"прям., перен. закали́ться, закаля́ться", "закали́ться"},
		{"грам. гла́сный", "гла́сный"},
		{"вы́стирать", "вы́стирать"},
	}
	for _, c := range cases {
		got := FirstGloss("дахча", []models.TranslationPairs{{
			Original: "дахча", Translate: c.gloss,
			OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD", Rate: 10000,
		}})
		if got != c.want {
			t.Errorf("FirstGloss(%q) = %q, want %q", c.gloss, got, c.want)
		}
	}
}

// «тоийта» is glossed «понуд. от тоа; прекрати́ть», and the card spent its
// first line on a word the reader neither asked about nor got translated.
func TestCard_PointerSensesComeLast(t *testing.T) {
	body := Render("тоийта", []models.TranslationPairs{
		{Original: "тоийта", Translate: "понуд. от тоа", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"},
		{Original: "тоийта", Translate: "прекрати́ть", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"},
	}).Body
	if !strings.Contains(body, "1. прекрати́ть") {
		t.Errorf("the meaning is not the first thing on the card:\n%s", body)
	}
	if !strings.Contains(body, "понуд. от тоа") {
		t.Errorf("the derivation was dropped rather than moved:\n%s", body)
	}

	// With nothing to move behind, the pointer stays — it is what CrossRef and
	// DerivedFrom read to find the word that does get translated.
	body = Render("ваха", []models.TranslationPairs{
		{Original: "ваха", Translate: "см. даха", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"},
	}).Body
	if !strings.Contains(body, "см. даха") {
		t.Errorf("a card that is only a pointer lost its pointer:\n%s", body)
	}
}

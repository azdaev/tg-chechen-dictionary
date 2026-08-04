package tools

import (
	"chetoru/internal/models"
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

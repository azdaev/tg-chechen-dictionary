package tools

import (
	"strings"
	"testing"

	"chetoru/internal/models"
)

// «Обрить» opens "сов, что [дӏа]даша" — the aspect label with a comma for its
// period. The label stripper wanted a period, so the whole chain stood in as
// the Chechen word: «обрить — сов, что дӏадаша».
func TestCard_AspectLabelWithAComma(t *testing.T) {
	cases := []struct{ query, gloss, want string }{
		{"обрить", "сов, что [дӏа]даша; ~ голову - корта баша", "дӏадаша"},
		{"пугать", "несов, кого кхеро; къахко (животное)", "кхеро"},
	}
	for _, c := range cases {
		card := FormatCard(c.query, []models.TranslationPairs{
			{Original: strings.ToUpper(c.query[:2]) + c.query[2:], Translate: c.gloss,
				OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, Rate: 100, EntryType: "WORD"},
		})
		if !strings.Contains(card, "<b>"+c.want+"</b>") {
			t.Errorf("%s: label survived as the translation:\n%s", c.query, card)
		}
	}
}

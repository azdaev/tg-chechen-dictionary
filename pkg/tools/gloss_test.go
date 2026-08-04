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

// The replacer scans longest-first so a short abbreviation is never found
// inside a longer one — but only for the ones it knows. «пр.» sat inside three
// it did not, and «с неопр.» reached the card as «с нео(предложный)».
func TestExpandAbbreviations_LongerOnesWin(t *testing.T) {
	cases := []struct{ in, want string }{
		{"чему и с неопр. ӏама", "чему и с (неопределённая форма) ӏама"},
		{"хьекхо (напр. одежду)", "хьекхо ((например) одежду)"},
		{"деепр. вприпры́жку", "(деепричастие) вприпры́жку"},
		{"в пр. падеже", "в (предложный) падеже"},
	}
	for _, c := range cases {
		if got := expandAbbreviations(c.in); got != c.want {
			t.Errorf("expandAbbreviations(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The bold on a card means Chechen and nothing else, and Russian government
// notation was reaching it: «идти» offered «куда (отправляться) даха» and
// «тӏекарчо» arrived as «на что тӏекарчо» — 7 of 489 bold strings across the
// sampled cards were a Russian preposition wearing the mark for Chechen.
func TestCard_RussianGovernmentIsNotChechen(t *testing.T) {
	cases := []struct{ gloss, want string }{
		{"на что тӏекарчо", "тӏекарчо"},
		{"во что чуӏотта, чудолла", "чуӏотта, чудолла"},
		{"за кем-чем, (переносное) новкъа даха", "(переносное) новкъа даха"},
		{"куда (отправляться) даха", "(отправляться) даха"},
		// «куда» opens Russian phrases too, and those are somebody's
		// translation, not a label: only the one introducing a qualifier goes.
		{"куда угодно", "куда угодно"},
		{"латта", "латта"},
	}
	for _, c := range cases {
		if got := stripLabels(c.gloss); got != c.want {
			t.Errorf("stripLabels(%q) = %q, want %q", c.gloss, got, c.want)
		}
	}
}

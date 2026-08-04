package tools

import (
	"chetoru/internal/models"
	"testing"
)

func TestEscapeUnclosedTags(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text untouched", "цӏа дитт", "цӏа дитт"},
		{"balanced tags untouched", "<b>дитт</b>", "<b>дитт</b>"},
		{"unclosed tag cleaned", "<b>дитт", "дитт"},
		{"stray opening bracket dropped", "цӏа < дитт", "цӏа  дитт"},
		{"stray closing bracket dropped", "цӏа > дитт", "цӏа  дитт"},
	}
	for _, c := range cases {
		if got := EscapeUnclosedTags(c.in); got != c.want {
			t.Errorf("%s: EscapeUnclosedTags(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestFormatPairs_TamesRawBrackets(t *testing.T) {
	// Local DB pairs carry raw content; a stray bracket must not survive into
	// the HTML-mode message. The card now emits deliberate tags of its own, so
	// "contains no angle bracket" no longer separates the two — assert the whole
	// line instead: the bold the renderer meant to write, and nothing the data
	// smuggled in.
	got := FormatPairs([]models.TranslationPairs{{Original: "цӏа <", Translate: "дом"}})
	if want := "<b>цӏа </b> — дом"; got != want {
		t.Errorf("formatted card = %q, want %q", got, want)
	}
}

func TestNormalizeSearch_FoldsPalochka(t *testing.T) {
	// All common ways to type the palochka must normalize identically: the
	// real letter (both cases), digit 1, and Latin i/l.
	want := NormalizeSearch("гӏала")
	for _, v := range []string{"г1ала", "гӀала", "гIала", "гlала"} {
		if got := NormalizeSearch(v); got != want {
			t.Errorf("NormalizeSearch(%q) = %q, want %q", v, got, want)
		}
	}

	// Standalone Latin words and numbers stay untouched.
	for _, v := range []string{"iphone", "123", "telegram"} {
		if got := NormalizeSearch(v); got != v {
			t.Errorf("NormalizeSearch(%q) = %q, want unchanged", v, got)
		}
	}

	if got := NormalizeSearch("1аж"); got != "ӏаж" {
		t.Errorf("NormalizeSearch(1аж) = %q, want ӏаж (leading stand-in folds)", got)
	}
	if got := NormalizeSearch("дег1"); got != "дегӏ" {
		t.Errorf("NormalizeSearch(дег1) = %q, want дегӏ (trailing stand-in folds)", got)
	}
}

func TestCleanTranslation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with leading dash",
			input:    "- дечиг",
			expected: "дечиг",
		},
		{
			name:     "with spaces",
			input:    "  уменьш. от рука  ",
			expected: "уменьш. от рука",
		},
		{
			name:     "normal text",
			input:    "ручка (для письма)",
			expected: "ручка (для письма)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanTranslation(tt.input)
			if result != tt.expected {
				t.Errorf("cleanTranslation() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// The separator needs a space on at least one side. Live glosses glue it to a
// preceding tilde, and the case-government shorthand never carries a space at
// all — so "surrounded by spaces" would break real records and "any dash"
// shreds real examples.
func TestSplitExample_DashGluedToTilde(t *testing.T) {
	cases := []struct {
		name, in, left, right string
		ok                    bool
	}{
		{"glued to the tilde", "деревянный ~- дечиган цӏа", "деревянный ~", "дечиган цӏа", true},
		{"glued on the right", "костюм ему ~ -костюм цунна йоккхо ю", "костюм ему ~", "костюм цунна йоккхо ю", true},
		{"spaces on both sides", "~ путь - беха некъ", "~ путь", "беха некъ", true},
		{"en dash glued right", "из глаз ~ли слёзы –бӏаьргашкара хиш оьхура", "из глаз ~ли слёзы", "бӏаьргашкара хиш оьхура", true},
		// The whole point: "кого-л." is one token, not two sides.
		{"case shorthand is not a separator", "проводить кого-л. до дома", "", "", false},
		{"first qualifying dash wins", "что-л. взять - схьаэца", "что-л. взять", "схьаэца", true},
		{"no dash at all", "просто перевод", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			left, right, ok := splitExample(c.in)
			if ok != c.ok || left != c.left || right != c.right {
				t.Errorf("splitExample(%q) = %q/%q/%v, want %q/%q/%v", c.in, left, right, ok, c.left, c.right, c.ok)
			}
		})
	}
}

func TestReplaceTildeWithWord(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		word     string
		expected string
		exact    bool
	}{
		{
			name:     "bare tilde is the headword verbatim",
			text:     "фруктовое ~ → стоьмийн дитт",
			word:     "дерево",
			expected: "фруктовое дерево → стоьмийн дитт",
			exact:    true,
		},
		{
			// A tail longer than three letters is either an ending or a whole
			// word glued on, and nothing tells them apart: «~цами» of
			// «домочадцы» and «~мост» of «развести» are the same length and
			// opposite kinds. Measured over the live dictionary, eight of ten
			// such tails were endings, so guessing "separate word" printed
			// «взбеситься лась» far more often than it printed «дом культуры».
			// Not exact, so the caller drops the example.
			name:     "tilde glued to a long tail is not guessed",
			text:     "~культуры → культуран цӏа",
			word:     "дом",
			expected: "дом → культуран цӏа",
			exact:    false,
		},
		{
			// The same shape, the other kind: «собака ~лась» is «взбесилась».
			name:     "long tail that is an ending is not guessed either",
			text:     "собака ~лась",
			word:     "взбеситься",
			expected: "собака взбеситься",
			exact:    false,
		},
		{
			name:     "vowel-final stem is regular",
			text:     "дверная ~а → наьӏаран тӏам; ~и дивана → диванан тӏаьмнаш",
			word:     "ручка",
			expected: "дверная ручка → наьӏаран тӏам; ручки дивана → диванан тӏаьмнаш",
			exact:    true,
		},
		{
			name:     "adjective stem is regular",
			text:     "~ое кричать",
			word:     "оглушительный",
			expected: "оглушительное кричать",
			exact:    true,
		},
		{
			// Fleeting vowel: «силки», not «силокки». Spelling does not say so.
			name:     "consonant-final headword is not derivable",
			text:     "ставить ~ки → хӏиттае",
			word:     "силок",
			expected: "ставить силок → хӏиттае",
			exact:    false,
		},
		{
			// The infinitive is not the present stem: «визжит», not «визжаит».
			name:     "verb infinitive is not derivable",
			text:     "щенок ~ит",
			word:     "визжать",
			expected: "щенок визжать",
			exact:    false,
		},
		{
			name:     "no tilde",
			text:     "обычный текст без тильды",
			word:     "слово",
			expected: "обычный текст без тильды",
			exact:    true,
		},
		{
			name:     "empty word",
			text:     "текст с ~ тильдой",
			word:     "",
			expected: "текст с ~ тильдой",
			exact:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, exact := replaceTildeWithWord(tt.text, tt.word)
			if result != tt.expected || exact != tt.exact {
				t.Errorf("replaceTildeWithWord() = %q/%v, want %q/%v", result, exact, tt.expected, tt.exact)
			}
		})
	}
}

func TestExpandAbbreviations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic abbreviations",
			input:    "дош; тж. къамел; разг. выражение",
			expected: "дош; также къамел; (разговорное) выражение",
		},
		{
			name:     "multiple abbreviations",
			input:    "уменьш. от слово; прост. говорить",
			expected: "уменьшительное от слово; (просторечие) говорить",
		},
		{
			name:     "technical abbreviations",
			input:    "мат. формула; физ. закон; хим. реакция",
			expected: "(математическое) формула; (физическое) закон; (химическое) реакция",
		},
		{
			name:     "grammatical abbreviations",
			input:    "род. падеж; тв. падеж; мн. число",
			expected: "(родительный) падеж; (творительный) падеж; (множественное) число",
		},
		{
			name:     "no abbreviations",
			input:    "обычный текст без сокращений",
			expected: "обычный текст без сокращений",
		},
		{
			name:     "partial matches should not replace",
			input:    "слово тж не должно заменяться",
			expected: "слово тж не должно заменяться",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandAbbreviations(tt.input)
			if result != tt.expected {
				t.Errorf("expandAbbreviations() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFirstExampleFor(t *testing.T) {
	// The example the card leads with is the example /wotd and /random show.
	// They used to mine the glosses separately, and the daily word shipped
	// without an example for every common word measured.
	pairs := []models.TranslationPairs{
		{Original: "Дом", Translate: "м цӏа; деревянный ~ - дечиган цӏа", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100},
	}
	got, ok := FirstExampleFor("дом", pairs)
	if !ok || got != "дечиган цӏа → деревянный дом" {
		t.Errorf("FirstExampleFor = %q/%v, want the card's own leading example", got, ok)
	}

	if _, ok := FirstExampleFor("дом", []models.TranslationPairs{
		{Original: "Дом", Translate: "м цӏа", OriginalLang: "RUS", TranslateLang: "CHE", Packed: true, EntryType: "WORD", Rate: 100},
	}); ok {
		t.Error("an entry with no examples reported one")
	}
}

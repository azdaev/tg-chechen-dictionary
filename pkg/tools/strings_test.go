package tools

import (
	"strings"
	"testing"
)

// TestFormatDeterminism guards against non-deterministic output. Go randomizes
// map iteration order on every range (even within one process), so if any code
// path's result depended on map order, repeated runs on the same input would
// eventually disagree. We use a gloss rich in abbreviations (which exercise the
// expandAbbreviations map) and tildes, and assert byte-identical output.
func TestFormatDeterminism(t *testing.T) {
	// Real-style gloss: multiple meanings, abbreviations (some substrings of
	// others, e.g. "им." inside "хим."), and tilde examples.
	input := "**дом** м 1) хим. им. род. цӏа; ~культуры -культуран цӏа; ~ отдыха – садаӏаран цӏа 2) (учреждение) тех. ист. цӏа; детский ~- берийн цӏа"
	word := "дом"

	first := FormatTranslationLite(input, word)
	for i := range 200 {
		if got := FormatTranslationLite(input, word); got != first {
			t.Fatalf("FormatTranslationLite non-deterministic at iter %d:\n first=%q\n got  =%q", i, first, got)
		}
	}

	abbrevInput := "хим. им. род. дат. вин. тв. пр. мн. ед. тех. мед. юр."
	firstAbbrev := expandAbbreviations(abbrevInput)
	for i := range 200 {
		if got := expandAbbreviations(abbrevInput); got != firstAbbrev {
			t.Fatalf("expandAbbreviations non-deterministic at iter %d:\n first=%q\n got  =%q", i, firstAbbrev, got)
		}
	}
}

func TestFormatTranslationLite_AdjectiveGloss(t *testing.T) {
	// Real «Домашний» entry: adjective ending list ("-яя, -ее"), palochka
	// standing in for the "1." sense marker, dot-style "2." marker, and a
	// tilde inside a sense line (not just in examples).
	input := "**Домашний** -яя, -ее ӏ. цӏера; ~ий адрес – цӏера адрес 2. в знач. сущ. ~ие мн. цӏеранаш"
	got := FormatTranslationLite(input, "Домашний")

	if !strings.HasPrefix(got, "Домашний — цӏера") {
		t.Errorf("header = %q, want it to start with %q (endings and ӏ. stripped)", got, "Домашний — цӏера")
	}
	if strings.Contains(got, "яя") || strings.Contains(got, "ӏ.") {
		t.Errorf("endings or palochka marker leaked into output:\n%s", got)
	}
	if !strings.Contains(got, "в знач. сущ. домашние") {
		t.Errorf("second sense missing or tilde unreplaced:\n%s", got)
	}
	if !strings.Contains(got, "домашний адрес → цӏера адрес") {
		t.Errorf("example not rendered:\n%s", got)
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

func TestParseExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "single example",
			input: "самопишущая ~а - ша язден ручка",
			expected: []string{
				"самопишущая ~а → ша язден ручка",
			},
		},
		{
			name:  "multiple examples with semicolons",
			input: "самопишущая ~а - ша язден ручка; шариковая ~а - шарикан ручка; ~а с пером - перо йолу ручка",
			expected: []string{
				"самопишущая ~а → ша язден ручка",
				"шариковая ~а → шарикан ручка",
				"~а с пером → перо йолу ручка",
			},
		},
		{
			// Live dosham glosses mix hyphens with en/em dashes as the separator
			// ("~ отдыха – садаӏаран цӏа" in the real «Дом» entry).
			name:  "en-dash and em-dash separators",
			input: "~ отдыха – садаӏаран цӏа; ~ моды — мода цӏа",
			expected: []string{
				"~ отдыха → садаӏаран цӏа",
				"~ моды → мода цӏа",
			},
		},
		{
			name:  "complex sentence example",
			input: "ас сайн ахча цуьнгахь дитина я оставил у него свои деньги",
			expected: []string{
				"ас сайн ахча цуьнгахь дитина я оставил у него свои деньги",
			},
		},
		{
			name:     "no examples",
			input:    "просто текст без примеров",
			expected: []string{"просто текст без примеров"},
		},
		{
			name:  "more than 5 examples should be limited",
			input: "ex1 - пер1; ex2 - пер2; ex3 - пер3; ex4 - пер4; ex5 - пер5; ex6 - пер6; ex7 - пер7",
			expected: []string{
				"ex1 → пер1",
				"ex2 → пер2",
				"ex3 → пер3",
				"ex4 → пер4",
				"ex5 → пер5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseExamples(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseExamples() returned %d examples, want %d", len(result), len(tt.expected))
				t.Errorf("got: %v", result)
				t.Errorf("want: %v", tt.expected)
				return
			}

			for i, expected := range tt.expected {
				if i >= len(result) || result[i] != expected {
					t.Errorf("parseExamples()[%d] = %q, want %q", i, result[i], expected)
				}
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
	}{
		{
			name:     "simple tilde replacement",
			text:     "фруктовое ~ → стоьмийн дитт",
			word:     "дерево",
			expected: "фруктовое дерево → стоьмийн дитт",
		},
		{
			name:     "tilde with ending",
			text:     "самопишущая ~а → ша язден ручка",
			word:     "ручка",
			expected: "самопишущая ручка → ша язден ручка",
		},
		{
			name:     "multiple tildes with different endings",
			text:     "дверная ~а → наьӏаран тӏам; ~и дивана → диванан тӏаьмнаш",
			word:     "ручка",
			expected: "дверная ручка → наьӏаран тӏам; ручки дивана → диванан тӏаьмнаш",
		},
		{
			name:     "tilde with complex endings",
			text:     "говорить о ~е; в ~ах; ~ами",
			word:     "слово",
			expected: "говорить о слове; в словах; словами",
		},
		{
			// Real dosham data: the "Дом" gloss glues a whole word to the tilde
			// ("~культуры"). It must read "дом культуры", not "домкультуры".
			name:     "tilde glued to a separate word",
			text:     "~культуры → культуран цӏа",
			word:     "дом",
			expected: "дом культуры → культуран цӏа",
		},
		{
			name:     "no tilde",
			text:     "обычный текст без тильды",
			word:     "слово",
			expected: "обычный текст без тильды",
		},
		{
			name:     "empty word",
			text:     "текст с ~ тильдой",
			word:     "",
			expected: "текст с ~ тильдой",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceTildeWithWord(tt.text, tt.word)
			if result != tt.expected {
				t.Errorf("replaceTildeWithWord() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetWordBase(t *testing.T) {
	tests := []struct {
		word     string
		expected string
	}{
		{"ручка", "ручк"},
		{"дерево", "дерев"},
		{"стол", "стол"},
		{"дом", "дом"},
		{"мама", "мам"},
		{"папа", "пап"},
		{"", ""},
		{"а", "а"},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := getWordBase(tt.word)
			if result != tt.expected {
				t.Errorf("getWordBase(%q) = %q, want %q", tt.word, result, tt.expected)
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

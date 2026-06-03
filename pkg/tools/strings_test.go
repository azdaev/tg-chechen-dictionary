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
	for i := 0; i < 200; i++ {
		if got := FormatTranslationLite(input, word); got != first {
			t.Fatalf("FormatTranslationLite non-deterministic at iter %d:\n first=%q\n got  =%q", i, first, got)
		}
	}

	abbrevInput := "хим. им. род. дат. вин. тв. пр. мн. ед. тех. мед. юр."
	firstAbbrev := expandAbbreviations(abbrevInput)
	for i := 0; i < 200; i++ {
		if got := expandAbbreviations(abbrevInput); got != firstAbbrev {
			t.Fatalf("expandAbbreviations non-deterministic at iter %d:\n first=%q\n got  =%q", i, firstAbbrev, got)
		}
	}
}

func TestFormatTranslation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "api карандаш",
			input: "**Карандаш** - м къолам; химический ~ - шекъа долун къолам; цветные ~и - бес-бесара къоламаш",
			expected: `📝 КАРАНДАШ

1️⃣ къолам
   • химический карандаш → шекъа долун къолам
   • цветные карандаши → бес-бесара къоламаш`,
		},
		{
			name:     "api маьл",
			input:    "**Маьлказ** - клен",
			expected: "📝 МАЬЛКАЗ\n\n1️⃣ клен",
		},
		{
			name:     "api мел",
			input:    "**Мел** - мест. 1) (о неопределенном количестве) сколько;",
			expected: "📝 МЕЛ\n\n1️⃣ (о неопределенном количестве) сколько",
		},
		{
			name:     "api хедар",
			input:    "**Хедар** - ӏ блюдо (посуда)",
			expected: "📝 ХЕДАР\n\n1️⃣ ӏ блюдо (посуда)",
		},
		{
			name:     "api тергал",
			input:    "**Тергал ве со** - обрати на меня внимание",
			expected: "📝 ТЕРГАЛ ВЕ СО\n\n1️⃣ обрати на меня внимание",
		},
		{
			name:     "api къолам",
			input:    "**Къолам** - карандаш",
			expected: "📝 КЪОЛАМ\n\n1️⃣ карандаш",
		},
		{
			name:     "api седалище",
			input:    "**Седалище** - с гаъ",
			expected: "📝 СЕДАЛИЩЕ\n\n1️⃣ гаъ",
		},
		{
			name:     "api дош",
			input:    "**Дошло** - кавалерист; всадник",
			expected: "📝 ДОШЛО\n\n1️⃣ кавалерист\n   • всадник",
		},
		{
			name:  "simple single meaning",
			input: "**ручка** - ручка (для письма)",
			expected: `📝 РУЧКА

1️⃣ ручка (для письма)`,
		},
		{
			name: "complex entry with multiple meanings",
			input: `**Ручка** - ж 1) уменьш. от рука; 2) (для письма) ручка; самопишущая ~а - ша язден ручка; шариковая ~а - шарикан ручка; ~а с пером - перо йолу ручка 3) (посуды, прибора, мебели) тӏам; (инструмента) мукъ; (ведра, котла) кӏай; дверная ~а - наьӏаран тӏам; ~а ножа - уьрсан мукъ; ~и дивана- диванан тӏаьмнаш; без ручек- тӏам боцуш ; дойти до ~и прост. - дан хӏума доцчу дала (кхача)`,
			expected: `📝 РУЧКА

1️⃣ уменьшительное от рука
2️⃣ (для письма) ручка
   • самопишущая ручка → ша язден ручка
   • шариковая ручка → шарикан ручка
   • ручка с пером → перо йолу ручка

3️⃣ (посуды, прибора, мебели) тӏам
   • (инструмента) мукъ
   • (ведра, котла) кӏай
   • дверная ручка → наьӏаран тӏам
   • ручка ножа → уьрсан мукъ
   • ручки дивана → диванан тӏаьмнаш`,
		},
		{
			name: "tree entry with examples",
			input: `**Дерево** - с 1) дитт; фруктовое ~ - стоьмийн дитт; лиственное ~ - гӏаш долу дитт 2) (материал) дечиг; стол красного дерева – цӏечу дечиган стол ; родословное ~ -силсил`,
			expected: `📝 ДЕРЕВО

1️⃣ дитт
   • фруктовое дерево → стоьмийн дитт
   • лиственное дерево → гӏаш долу дитт

2️⃣ (материал) дечиг
   • стол красного дерева – цӏечу дечиган стол
   • родословное дерево → силсил`,
		},
		{
			name: "verb entry",
			input: `**Дита** - 1) в разн. знач. оставить, покинуть; ас сайн ахча цуьнгахь дитина я оставил у него свои деньги; 2) развестись, расторгнуть брак; цо зуда йитина он развелся с женой;  мостагӏ вита простить врагу (т. е. оставить без мщения); цигаьрка йита бросить курить (букв, оставить папиросу); мекхаш дита оставить усы (т. е. не брить усов)`,
			expected: `📝 ДИТА

1️⃣ в разн. знач. оставить, покинуть
   • ас сайн ахча цуьнгахь дитина я оставил у него свои деньги

2️⃣ развестись, расторгнуть брак
   • цо зуда йитина он развелся с женой
   • мостагӏ вита простить врагу (т. е. оставить без мщения)
   • цигаьрка йита бросить курить (букв, оставить папиросу)
   • мекхаш дита оставить усы (т. е. не брить усов)`,
		},
		{
			name: "entry without bold word",
			input: `дечиг-пхьолин; ~ые работы - дечиг-пхьолин белхаш`,
			expected: `1️⃣ дечиг-пхьолин
   • ~ые работы → дечиг-пхьолин белхаш`,
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name: "single meaning without numbering",
			input: `**Детта** - доить; етт бетта доить корову`,
			expected: `📝 ДЕТТА

1️⃣ доить
   • етт бетта доить корову`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTranslation(tt.input)

			// Normalize whitespace for comparison
			normalizeWhitespace := func(s string) string {
				return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
			}

			expected := normalizeWhitespace(tt.expected)
			actual := normalizeWhitespace(result)

			if actual != expected {
				t.Errorf("FormatTranslation() =\n%q\nwant:\n%q", actual, expected)
			}
		})
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

func TestGetNumberEmoji(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{1, "1️⃣"},
		{2, "2️⃣"},
		{3, "3️⃣"},
		{10, "🔟"},
		{11, "11️⃣"},
		{0, "0️⃣"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := getNumberEmoji(tt.input)
			if result != tt.expected {
				t.Errorf("getNumberEmoji(%d) = %q, want %q", tt.input, result, tt.expected)
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

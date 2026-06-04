package business

import (
	"chetoru/internal/models"
	"slices"
	"testing"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"ce":   "CHE", // new API ISO code
		"CE":   "CHE",
		"che":  "CHE",
		"CHE":  "CHE", // legacy code still accepted
		"ru":   "RUS",
		"RU":   "RUS",
		"rus":  "RUS",
		"RUS":  "RUS",
		" ce ": "CHE", // trimmed
		"en":   "",    // other languages dropped
		"ar":   "",
		"tr":   "",
		"":     "",
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInferOriginalLang(t *testing.T) {
	cases := map[string]string{
		"RUS": "CHE",
		"CHE": "RUS",
		"en":  "",
		"":    "",
	}
	for in, want := range cases {
		if got := inferOriginalLang(in); got != want {
			t.Errorf("inferOriginalLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLearnableWord(t *testing.T) {
	learnable := []string{"къолам", "дитт", "ӀаьржакӀа", "дечиг пхьар"}
	for _, w := range learnable {
		if !isLearnableWord(w) {
			t.Errorf("isLearnableWord(%q) = false, want true", w)
		}
	}

	notLearnable := []string{
		"с дашар; ~ бороды", // example clause
		"~ое работы",        // tilde placeholder
		"(материал) дечиг",  // parenthetical marker
		"мел 1) сколько",    // numbered sense
		"очень длинное чеченское словосочетание", // too long / many words
		"", // empty
		// Real /random bug: a Russian-headword adjective gloss leaked through.
		"-ая, -ое 1) лекха; ~ие горы- лекха лаьмнаш; ~ий человек - лекха стаг",
		"-ая, -ое",            // leading grammatical-ending marker
		"раскрытие, открытие", // comma-separated gloss, not a single word
	}
	for _, w := range notLearnable {
		if isLearnableWord(w) {
			t.Errorf("isLearnableWord(%q) = true, want false", w)
		}
	}
}

func TestIsCleanMeaning(t *testing.T) {
	clean := []string{"палач", "плавка", "раскрытие, открытие", "тот, та, то"}
	for _, m := range clean {
		if !isCleanMeaning(m) {
			t.Errorf("isCleanMeaning(%q) = false, want true", m)
		}
	}

	dirty := []string{
		"см. тезалла",   // cross-reference
		"дашар; пример", // multi-clause
		"~ что-то",      // tilde
		"очень длинное и подробное толкование которое не помещается", // too long
		"",    // empty
		"   ", // whitespace only
		// Derivational cross-references — abbreviation period rejects these.
		"понуд. от самукъадаккха",
		"масд. от сагатдан",
		"прил. коло",
	}
	for _, m := range dirty {
		if isCleanMeaning(m) {
			t.Errorf("isCleanMeaning(%q) = true, want false", m)
		}
	}
}

func TestStripLeadingGenderMarker(t *testing.T) {
	cases := map[string]string{
		"ж астагӏалла": "астагӏалла", // strip feminine marker
		"м комбинезон": "комбинезон", // strip masculine marker
		"с тешнабехк":  "тешнабехк",  // strip neuter marker
		"мн чоьташ":    "чоьташ",     // strip plural marker
		"астагӏалла":   "астагӏалла", // no marker -> unchanged
		"маршо":        "маршо",      // first letter isn't a marker token
		"ж":            "ж",          // marker alone -> leave (nothing follows)
	}
	for in, want := range cases {
		if got := stripLeadingGenderMarker(in); got != want {
			t.Errorf("stripLeadingGenderMarker(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrientEntry(t *testing.T) {
	// content is Chechen, translation is Russian -> Chechen = content.
	ru := models.Entry{
		Content:      "дитт",
		Translations: []models.Translation{{Content: "дерево", LanguageCode: "ru"}},
	}
	if w := orientEntry(ru); w == nil || w.Chechen != "дитт" || w.Russian != "дерево" {
		t.Errorf("orientEntry(ru) = %+v, want {дитт, дерево}", w)
	}

	// content is Russian, translation is Chechen -> Chechen = translation.
	ce := models.Entry{
		Content:      "Дерево",
		Translations: []models.Translation{{Content: "дитт", LanguageCode: "ce"}},
	}
	if w := orientEntry(ce); w == nil || w.Chechen != "дитт" || w.Russian != "Дерево" {
		t.Errorf("orientEntry(ce) = %+v, want {дитт, Дерево}", w)
	}

	// Russian reading is preferred when both exist (cleaner Chechen headword).
	both := models.Entry{
		Content: "цӏа",
		Translations: []models.Translation{
			{Content: "дом", LanguageCode: "ru"},
			{Content: "house", LanguageCode: "en"},
		},
	}
	if w := orientEntry(both); w == nil || w.Chechen != "цӏа" || w.Russian != "дом" {
		t.Errorf("orientEntry(both) = %+v, want {цӏа, дом}", w)
	}

	// No Russian/Chechen translation -> nil.
	none := models.Entry{
		Content:      "test",
		Translations: []models.Translation{{Content: "test", LanguageCode: "en"}},
	}
	if w := orientEntry(none); w != nil {
		t.Errorf("orientEntry(none) = %+v, want nil", w)
	}
}

func TestMakeRandomWord(t *testing.T) {
	// Strips HTML tags and trims.
	if w := makeRandomWord("  <b>дитт</b> ", " дерево "); w == nil || w.Chechen != "дитт" || w.Russian != "дерево" {
		t.Errorf("makeRandomWord cleaning = %+v, want {дитт, дерево}", w)
	}
	// Empty side -> nil.
	if w := makeRandomWord("", "дерево"); w != nil {
		t.Errorf("makeRandomWord(empty, _) = %+v, want nil", w)
	}
	if w := makeRandomWord("дитт", "   "); w != nil {
		t.Errorf("makeRandomWord(_, blank) = %+v, want nil", w)
	}
}

func TestHasExactOriginal(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Белка", Translate: "тарсал"},
		{Original: "Ёлка", Translate: "база"},
	}
	if !hasExactOriginal(pairs, "елка") {
		t.Error("елка should match Ёлка (ё/е-insensitive)")
	}
	if !hasExactOriginal(pairs, "БЕЛКА") {
		t.Error("БЕЛКА should match Белка (case-insensitive)")
	}
	if hasExactOriginal(pairs, "лка") {
		t.Error("substring must not count as exact match")
	}
	if hasExactOriginal(nil, "елка") {
		t.Error("no pairs can't contain a match")
	}
}

func TestMergePairs(t *testing.T) {
	first := []models.TranslationPairs{
		{Original: "Ёлка", Translate: "база"},
	}
	second := []models.TranslationPairs{
		{Original: "Белка", Translate: "тарсал"},
		{Original: "ёлка", Translate: "База"}, // duplicate of first, modulo case/ё
	}
	got := mergePairs(first, second)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged pairs, got %d: %+v", len(got), got)
	}
	if got[0].Original != "Ёлка" || got[1].Original != "Белка" {
		t.Errorf("alternate results must come first, got %+v", got)
	}
}

func TestPrefixCandidates(t *testing.T) {
	cases := map[string][]string{
		"яблоками":  {"яблокам", "яблока", "яблок", "ябло"},
		"воды":      {"вод"}, // stops at minSuggestPrefix
		"дом":       nil,     // too short to trim
		"два слова": nil,     // phrases are not trimmed
		"":          nil,
	}
	for in, want := range cases {
		if got := prefixCandidates(in); !slices.Equal(got, want) {
			t.Errorf("prefixCandidates(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterPrefixMatches(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "Ӏаж"},
		{Original: "Дозревать", Translate: "кхиа"}, // substring hit, not a prefix
		{Original: "Ӏаж", Translate: "яблоко"},     // prefix on the translate side
	}
	got := filterPrefixMatches(pairs, "яблок")
	if len(got) != 2 || got[0].Original != "Яблоко" || got[1].Translate != "яблоко" {
		t.Errorf("filterPrefixMatches = %+v, want Яблоко and яблоко", got)
	}
	if got := filterPrefixMatches(pairs, "груш"); len(got) != 0 {
		t.Errorf("expected no matches for груш, got %+v", got)
	}
}

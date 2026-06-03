package business

import (
	"chetoru/internal/models"
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
	}
	for _, m := range dirty {
		if isCleanMeaning(m) {
			t.Errorf("isCleanMeaning(%q) = true, want false", m)
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

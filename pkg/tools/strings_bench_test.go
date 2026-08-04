package tools

import (
	"chetoru/internal/models"
	"testing"
)

func BenchmarkClean(b *testing.B) {
	for b.Loop() {
		Clean("<b>дитт</b> — дерево<br />и ещё <i>что-то</i>")
	}
}

func BenchmarkCleanPlain(b *testing.B) {
	for b.Loop() {
		Clean("дитт — дерево, обычная словарная строка без разметки")
	}
}

func BenchmarkCard(b *testing.B) {
	pairs := []models.TranslationPairs{{
		Original:      "Чёрный",
		Translate:     "-ая, -ое 1) Ӏаьржа; ~ое море - Ӏаьржа хӀорд; перен. ~ день - вон де 2) разг. сийна; ~ хлеб - сийна бепиг",
		OriginalLang:  "RUS",
		TranslateLang: "CHE",
		EntryType:     "WORD",
		Rate:          100,
	}}
	for b.Loop() {
		FormatCard("чёрный", pairs)
	}
}

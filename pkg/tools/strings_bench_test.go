package tools

import "testing"

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

func BenchmarkFormatTranslationLite(b *testing.B) {
	const entry = "**черный** -ая, -ое 1) Ӏаьржа; ~ое море - Ӏаьржа хӀорд; перен. ~ день - вон де 2) разг. сийна; ~ хлеб - сийна бепиг"
	for b.Loop() {
		FormatTranslationLite(entry, "черный", true)
	}
}

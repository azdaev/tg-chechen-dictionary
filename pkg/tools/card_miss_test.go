package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

// «лоьма» matched «Лоьма-кӏорца» and «лоьманиг» by prefix and nothing else. The
// bot used to send that footer alone and count it as an answer, so the word
// never reached the missing-words report.
func TestCard_NeighboursAloneAreNotAnAnswer(t *testing.T) {
	body, neighbours := Card("лоьма", []models.TranslationPairs{
		{Original: "Лоьма-кӏорца", Translate: "лем-Корц", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
		{Original: "лоьманиг", Translate: "львиный", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if body != "" {
		t.Fatalf("neighbours rendered as a card body:\n%s", body)
	}
	if len(neighbours) != 2 {
		t.Fatalf("neighbours = %q, want both kept for the miss hint", neighbours)
	}
	if FormatCard("лоьма", []models.TranslationPairs{
		{Original: "лоьманиг", Translate: "львиный", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	}) != "" {
		t.Error("FormatCard still returns a neighbours-only card")
	}
}

// «стрим» matched inside «гольфстрим» and produced no card at all. The handler
// used to paper over that with a raw pair dump.
func TestCard_BodyOnlyMentionIsNotAnAnswer(t *testing.T) {
	body, neighbours := Card("стрим", []models.TranslationPairs{
		{Original: "Течение гольфстрим хи.", Translate: "гольфстрим", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "TEXT"},
	})
	if body != "" || len(neighbours) != 0 {
		t.Fatalf("a substring mention became an answer: body=%q neighbours=%q", body, neighbours)
	}
}

// The academic corpus writes stress, the others do not. Both spellings reached
// the card as separate senses: "1. телефо́н 2. телефон".
func TestCard_StressVariantsAreOneSense(t *testing.T) {
	card := FormatCard("телефон", []models.TranslationPairs{
		{Original: "телефон", Translate: "телефо́н", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD"},
		{Original: "телефон", Translate: "телефон", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if strings.Contains(card, "1.") {
		t.Fatalf("one word rendered as two senses:\n%s", card)
	}
}

// The Russian–Chechen articles store capitalized headwords and the other three
// corpora do not, so «Карандаш» and «телефон» came out of one lookup shape.
func TestCard_HeadwordsAreLowercase(t *testing.T) {
	card := FormatCard("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.HasPrefix(card, "карандаш") {
		t.Fatalf("headword kept the source capitalization:\n%s", card)
	}
}

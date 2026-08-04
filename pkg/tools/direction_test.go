package tools

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

// Bold was the only thing marking the Chechen side, and the card never said so.
// Asked for a loanword the bot answered «телефон → 1. телефон» and there was no
// way to tell which of the two was the Chechen one.
func TestCard_EveryBlockNamesItsDirection(t *testing.T) {
	che, _ := Card("къолам", []models.TranslationPairs{
		{Original: "къолам", Translate: "карандаш", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 16, EntryType: "WORD"},
	})
	if !strings.Contains(che, "чеч. → рус.") {
		t.Errorf("Chechen lookup did not say which side is Chechen:\n%s", che)
	}

	rus, _ := Card("карандаш", []models.TranslationPairs{
		{Original: "Карандаш", Translate: "м къолам", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
	})
	if !strings.Contains(rus, "рус. → чеч.") {
		t.Errorf("Russian lookup did not say which side is Chechen:\n%s", rus)
	}
}

// «лом» is a Russian crowbar and a Chechen lion. Both spellings keyed the same
// block, so the card listed «лев» among the Chechen translations of «лом» —
// telling the user that the Chechen for «лом» is «лев».
func TestCard_CrossLanguageHomographsDoNotMerge(t *testing.T) {
	body, _ := Card("лом", []models.TranslationPairs{
		{Original: "Лом", Translate: "м лом, ваба", OriginalLang: "RUS", TranslateLang: "CHE", Rate: 100, EntryType: "WORD"},
		{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", Rate: 10000, EntryType: "WORD", EntryIndex: 1},
	})
	for _, para := range strings.Split(body, "\n\n") {
		if strings.Contains(para, "ваба") && strings.Contains(para, "лев") {
			t.Fatalf("both readings landed in one block:\n%s", body)
		}
	}
	if !strings.Contains(body, "чеч. → рус.") || !strings.Contains(body, "рус. → чеч.") {
		t.Errorf("the two readings are not labelled apart:\n%s", body)
	}
}

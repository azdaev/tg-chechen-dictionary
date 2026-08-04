package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"testing"

	"github.com/sirupsen/logrus"
)

// End to end through the layer that actually reaches the user: business finds
// the word, tools.Card must still render it. Every spelling-tolerance layer had
// been dead here — business resolved the word and the renderer threw it away.
func TestTranslateResolved_RendersACard(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"чегардиг": true, "лоьман": true}}
	probe.start(t)

	repo := &formsDictRepo{
		byWord: map[string][]models.TranslationPairs{
			"лом": {{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS", EntryType: "WORD"}},
		},
		byForm: map[string][]string{tools.FoldSearch("ло̃ьман"): {"лом"}},
	}
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}

	pairs, resolved, err := b.TranslateResolved("лоьман")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("the form did not resolve at all")
	}
	if resolved != "лом" {
		t.Fatalf("resolved = %q, want the lemma лом so the card can be keyed on it", resolved)
	}
	key := resolved
	if key == "" {
		key = "лоьман"
	}
	if body := tools.Render(key, pairs).Body; body == "" {
		t.Fatalf("business resolved the form and the renderer dropped it: %+v", pairs)
	}
}

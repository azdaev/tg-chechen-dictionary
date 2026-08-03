package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
)

type formsDictRepo struct {
	recordingDictRepo
	byWord  map[string][]models.TranslationPairs
	byForm  map[string][]string
	mu      sync.Mutex
	written map[string][]string
}

func (r *formsDictRepo) FindTranslationPairs(_ context.Context, cleanWord string, _ int) ([]models.TranslationPairs, error) {
	return r.byWord[cleanWord], nil
}

func (r *formsDictRepo) FindHeadwordsByForm(_ context.Context, folded string, _ int) ([]string, error) {
	return r.byForm[folded], nil
}

func (r *formsDictRepo) SaveWordForms(_ context.Context, headword string, forms []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.written == nil {
		r.written = map[string][]string{}
	}
	r.written[headword] = forms
	return nil
}

// The grammar card has been printing «Формы: ло̃ьман …» all along while a search
// for that form found nothing. Once the paradigm is indexed the form opens the
// lemma's card, and dosham is never asked.
func TestTranslate_InflectedFormOpensTheLemma(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"лоьман": true}}
	probe.start(t)

	repo := &formsDictRepo{
		byWord: map[string][]models.TranslationPairs{
			"лом": {{Original: "лом", Translate: "лев", OriginalLang: "CHE", TranslateLang: "RUS"}},
		},
		// Stored folded, which is how the tilde in «ло̃ьман» survives a user
		// typing «лоьман».
		byForm: map[string][]string{tools.FoldSearch("ло̃ьман"): {"лом"}},
	}
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}

	got, err := b.Translate("лоьман")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(got) != 1 || got[0].Original != "лом" {
		t.Fatalf("got %+v, want the lemma лом", got)
	}
	if n := probe.count("лоьман"); n != 0 {
		t.Fatalf("the API was queried %d times for a form we could already resolve", n)
	}
}

// A form nobody has indexed must not shortcut the cascade.
func TestTranslate_UnknownFormFallsThrough(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"цӏазам": true}}
	probe.start(t)

	repo := &formsDictRepo{byWord: map[string][]models.TranslationPairs{}, byForm: map[string][]string{}}
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: repo}

	if _, err := b.Translate("цӏазам"); err != nil {
		t.Fatalf("translate: %v", err)
	}
	if n := probe.count("цӏазам"); n == 0 {
		t.Fatal("an unknown form skipped the API instead of falling through")
	}
}

package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
)

func stubDoshamAPI(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOSHAM_API_URL", srv.URL)
}

func TestFetchTranslations_EmptyAnswerIsNotNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"data":{"find":[]}}`)
	b := &Business{log: logrus.New()}

	got := b.fetchTranslationsWithFallback("яблоками")
	if got == nil {
		t.Fatal("real empty answer must be non-nil so it gets negative-cached")
	}
	if len(got) != 0 {
		t.Fatalf("expected no pairs, got %d", len(got))
	}
}

// The renderer bolds the Chechen side and orders examples by it, so the API
// mapper has to say which side that is — the entry itself carries no marker.
func TestFetchTranslations_CarriesLanguages(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"data":{"find":[
		{"entryId":"1","content":"Дом","type":"WORD","rate":100,
		 "translations":[{"translationId":"t1","content":"цӏа","languageCode":"ce"}]}]}}`)
	b := &Business{log: logrus.New()}

	got := b.fetchTranslationsWithFallback("Дом")
	if len(got) != 1 {
		t.Fatalf("got %d pairs, want 1", len(got))
	}
	if got[0].OriginalLang != "RUS" || got[0].TranslateLang != "CHE" {
		t.Errorf("langs = %q/%q, want RUS/CHE", got[0].OriginalLang, got[0].TranslateLang)
	}
}

func TestFetchTranslations_HTTPErrorIsNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusInternalServerError, `{"data":{"find":[]}}`)
	b := &Business{log: logrus.New()}

	if got := b.fetchTranslationsWithFallback("яблоками"); got != nil {
		t.Fatalf("HTTP failure must return nil so it is not cached, got %#v", got)
	}
}

// stubDoshamFind serves find() responses keyed by the searched word; unknown
// words get an empty result.
func stubDoshamFind(t *testing.T, entries map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				InputText string `json:"inputText"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		find := "[]"
		if headword, ok := entries[req.Variables.InputText]; ok {
			find = fmt.Sprintf(`[{"entryId":"e1","content":%q,"type":"WORD","translations":[{"translationId":"t1","content":"Ӏаж","languageCode":"che"}]}]`, headword)
		}
		fmt.Fprintf(w, `{"data":{"find":%s}}`, find)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOSHAM_API_URL", srv.URL)
}

func TestSuggestTranslations_LongestPrefixWins(t *testing.T) {
	// "яблоками" misses; prefixes "яблок" and "ябло" both have matches. The
	// longest one must win even though lookups run concurrently.
	stubDoshamFind(t, map[string]string{
		"яблок": "Яблоко",
		"ябло":  "Яблоня",
	})
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", "")}

	got := b.SuggestTranslations("яблоками")
	if len(got) != 1 || got[0].Original != "Яблоко" {
		t.Fatalf("suggestions = %+v, want exactly [Яблоко] from the longest prefix", got)
	}

	if got := b.SuggestTranslations("слово из фразы"); got != nil {
		t.Fatalf("phrases must not produce suggestions, got %+v", got)
	}
}

func TestFetchTranslations_YoFallbackFindsVariant(t *testing.T) {
	// "береза" itself misses; the ё-respelling "берёза" hits. Variant lookups
	// run concurrently, but the merged result must still surface the match.
	stubDoshamFind(t, map[string]string{"берёза": "Берёза"})
	b := &Business{log: logrus.New()}

	got := b.fetchTranslationsWithFallback("береза")
	if len(got) != 1 || got[0].Original != "Берёза" {
		t.Fatalf("translations = %+v, want exactly [Берёза]", got)
	}
}

func TestRecheckTranslation(t *testing.T) {
	stubDoshamFind(t, map[string]string{"яблоко": "Яблоко"})
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", "")}

	if !b.RecheckTranslation("яблоко") {
		t.Error("word now in the dictionary must report true")
	}
	if b.RecheckTranslation("ыыыы") {
		t.Error("still-missing word must report false")
	}
}

func TestSuggestTranslations_PhraseFallsBackToWords(t *testing.T) {
	// The phrase itself has no entry; each word does. Both words' top pairs
	// come back as suggestions, in phrase order.
	stubDoshamFind(t, map[string]string{"красное": "Красный", "яблоко": "Яблоко"})
	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", "")}

	got := b.SuggestTranslations("красное яблоко")
	if len(got) != 2 || got[0].Original != "Красный" || got[1].Original != "Яблоко" {
		t.Fatalf("suggestions = %+v, want [Красный Яблоко]", got)
	}
}

type recordingDictRepo struct {
	inserted chan repository.TranslationPair
}

func (r *recordingDictRepo) FindTranslationPairs(context.Context, string, int) ([]models.TranslationPairs, error) {
	return nil, nil
}

func (r *recordingDictRepo) FindTranslationPairsByPrefix(context.Context, string, int) ([]models.TranslationPairs, error) {
	return nil, nil
}

func (r *recordingDictRepo) InsertTranslationPair(_ context.Context, pair repository.TranslationPair) (int64, bool, error) {
	r.inserted <- pair
	return 1, true, nil
}

func (r *recordingDictRepo) UpdateTranslationPairFormatting(context.Context, int64, string, string) error {
	return nil
}

func (r *recordingDictRepo) SetTranslationPairFormattingChoice(context.Context, int64, string) error {
	return nil
}

func TestFetchTranslations_StoresPairsDetached(t *testing.T) {
	stubDoshamFind(t, map[string]string{"яблок": "Яблоко"})
	repo := &recordingDictRepo{inserted: make(chan repository.TranslationPair, 1)}
	b := &Business{log: logrus.New(), dictRepo: repo}

	got := b.fetchTranslationsFromAPI("яблок")
	if len(got) != 1 {
		t.Fatalf("translations = %+v, want 1 pair", got)
	}

	b.WaitBackground()
	select {
	case pair := <-repo.inserted:
		if pair.OriginalClean != "яблоко" || pair.TranslationLang != "CHE" {
			t.Fatalf("stored pair = %+v", pair)
		}
	default:
		t.Fatal("pair was never persisted")
	}
}

type prefixDictRepo struct {
	recordingDictRepo
	byPrefix map[string][]models.TranslationPairs
}

func (r *prefixDictRepo) FindTranslationPairsByPrefix(_ context.Context, prefix string, _ int) ([]models.TranslationPairs, error) {
	return r.byPrefix[prefix], nil
}

func TestSuggestTranslations_LocalDictionaryFirst(t *testing.T) {
	// The API is down; the local dictionary alone must still produce
	// suggestions, and the longest matching prefix wins.
	stubDoshamAPI(t, http.StatusInternalServerError, ``)
	repo := &prefixDictRepo{byPrefix: map[string][]models.TranslationPairs{
		"яблок": {{Original: "Яблоко", Translate: "Ӏаж"}},
		"ябло":  {{Original: "Яблоня", Translate: "Ӏаж дитт"}},
	}}
	b := &Business{log: logrus.New(), dictRepo: repo, cache: cache.NewCache("127.0.0.1:1", "")}

	got := b.SuggestTranslations("Яблоками")
	if len(got) != 1 || got[0].Original != "Яблоко" {
		t.Fatalf("suggestions = %+v, want [Яблоко] from the local dictionary", got)
	}
}

func TestFetchTranslations_GraphQLErrorIsNil(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"errors":[{"message":"internal error"}],"data":null}`)
	b := &Business{log: logrus.New()}

	if got := b.fetchTranslationsWithFallback("яблоками"); got != nil {
		t.Fatalf("GraphQL error must return nil so it is not cached, got %#v", got)
	}
}

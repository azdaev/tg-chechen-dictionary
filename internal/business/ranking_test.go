package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"context"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestRankAndDedup_NilStaysNil(t *testing.T) {
	// Translate negative-caches an empty non-nil answer for a day but never a
	// nil one — ranking must not convert a dosham outage into a cached miss.
	if got := rankAndDedup(nil, "яблоко"); got != nil {
		t.Fatalf("rankAndDedup(nil) = %+v, want nil", got)
	}
	empty := []models.TranslationPairs{}
	if got := rankAndDedup(empty, "яблоко"); got == nil {
		t.Fatal("rankAndDedup(empty) = nil, want a non-nil empty slice")
	}
}

func TestRankAndDedup_ExactMatchOutranksShorter(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Ӏаж", Translate: "яблоня"},
		{Original: "Ӏаж дитт", Translate: "яблоко"},
	}
	got := rankAndDedup(pairs, "яблоко")
	if got[0].Translate != "яблоко" {
		t.Fatalf("first result = %+v, want the exact translation match ahead of the shorter headword", got[0])
	}
}

func TestRankAndDedup_HigherRateWinsWithinBucket(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "редкий смысл", Rate: 1},
		{Original: "Яблоко", Translate: "основной смысл", Rate: 9},
	}
	got := rankAndDedup(pairs, "яблоко")
	if got[0].Translate != "основной смысл" {
		t.Fatalf("first result = %+v, want the higher-rated sense", got[0])
	}
}

func TestRankAndDedup_StableWithinEqualRank(t *testing.T) {
	// The local path gives every pair the same bucket and rate 0. The order
	// must still be identical across calls, because the first result freezes
	// into the cache and "Ещё" re-ranks on every press.
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "ба"},
		{Original: "Яблоко", Translate: "аа"},
		{Original: "Яблоко", Translate: "ва"},
	}
	first := rankAndDedup(append([]models.TranslationPairs(nil), pairs...), "яблоко")
	for range 20 {
		again := rankAndDedup(append([]models.TranslationPairs(nil), pairs...), "яблоко")
		for i := range first {
			if first[i].Translate != again[i].Translate {
				t.Fatalf("order drifted between calls: %+v vs %+v", first, again)
			}
		}
	}
}

func TestRankAndDedup_StressMarksAreOneEntry(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "я́блоко"},
		{Original: "Яблоко", Translate: "яблоко"},
	}
	if got := rankAndDedup(pairs, "яблоко"); len(got) != 1 {
		t.Fatalf("got %d pairs, want stress-only duplicates collapsed to 1: %+v", len(got), got)
	}
}

func TestRankAndDedup_PrefersAIFormattedDuplicate(t *testing.T) {
	// Whichever order dosham returns the duplicates in, the card must keep the
	// moderator-approved rendering.
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "Ӏаж"},
		{Original: "Яблоко", Translate: "Ӏаж", FormattedChosen: "ai", FormattedAI: "<b>Яблоко</b>"},
	}
	got := rankAndDedup(pairs, "яблоко")
	if len(got) != 1 || got[0].FormattedChosen != "ai" {
		t.Fatalf("got %+v, want the single AI-formatted pair", got)
	}
}

// The repository's ORDER BY puts moderated pairs first, but dedup leaves a
// total order behind, so nothing of the SQL order survives rankAndDedup. Both
// layers have to encode the preference or the moderator's work is invisible.
func TestRankAndDedup_ApprovedLeadsItsBucket(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Дитт", Translate: "Ясень", FormattedChosen: "ai", FormattedAI: "<b>Дитт</b> — ясень"},
		{Original: "Дитт", Translate: "Древо"},
		{Original: "Дитт", Translate: "Дерево"},
	}
	got := rankAndDedup(pairs, "дитт")
	if !approved(got[0]) {
		t.Fatalf("first result = %q, want the moderated pair ahead of the alphabet", got[0].Translate)
	}
}

func TestRankAndDedup_ShortestGlossWithinBucket(t *testing.T) {
	// Local-path shape: every headword is the query, so only the gloss varies.
	pairs := []models.TranslationPairs{
		{Original: "Дитт", Translate: "Дерево, растущее у дома"},
		{Original: "Дитт", Translate: "Древо"},
	}
	got := rankAndDedup(pairs, "дитт")
	if got[0].Translate != "Древо" {
		t.Fatalf("first result = %q, want the shortest gloss", got[0].Translate)
	}
}

// Duplicates carry their own rate. Keeping first-arrival would hand the choice
// to dosham's response order and throw the surviving pair's rate away before
// ranking ever reads it.
func TestRankAndDedup_HigherRatedDuplicateSurvives(t *testing.T) {
	pairs := []models.TranslationPairs{
		{Original: "Яблоко", Translate: "Ӏаж", Rate: 1},
		{Original: "яблоко", Translate: "ӏаж", Rate: 100},
		{Original: "Груша", Translate: "Кхор", Rate: 50},
	}
	got := rankAndDedup(pairs, "яблоко")
	if len(got) != 2 {
		t.Fatalf("got %d pairs, want the duplicate collapsed: %+v", len(got), got)
	}
	if got[0].Rate != 100 {
		t.Fatalf("surviving duplicate has rate %d, want the higher-rated one kept", got[0].Rate)
	}
}

type stubDictRepo struct {
	recordingDictRepo
	pairs []models.TranslationPairs
}

func (r *stubDictRepo) FindTranslationPairs(context.Context, string, int) ([]models.TranslationPairs, error) {
	return r.pairs, nil
}

func TestTranslate_LocalPathRankedAndDeduped(t *testing.T) {
	// The local table is the steady-state path once a word has been looked up
	// once, so ranking has to apply there and not only to API results.
	repo := &stubDictRepo{pairs: []models.TranslationPairs{
		{Original: "Яблоневый", Translate: "Ӏожан"},
		{Original: "Я́блоко", Translate: "Ӏаж"},
		{Original: "Яблоко", Translate: "Ӏаж"},
	}}
	b := &Business{log: logrus.New(), dictRepo: repo, cache: cache.NewCache("127.0.0.1:1", "")}

	got := b.Translate("яблоко")
	if len(got) != 2 {
		t.Fatalf("got %d pairs, want the stress duplicate collapsed: %+v", len(got), got)
	}
	if got[0].Translate != "Ӏаж" {
		t.Fatalf("first result = %+v, want the exact headword match", got[0])
	}
	b.WaitBackground()
}

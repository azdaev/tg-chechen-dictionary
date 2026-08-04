// Lookup pipeline: cache, then the local table, then its folded spellings, then
// the word-form index, then dosham, and last a lemma guessed from the stem.
// Each layer answers or falls through.
package business

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"

	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

// OnPairReady is called after a new pair is saved and AI formatting completes (or is skipped).
// pairID is the database ID, cleanWord is the normalized search term.
type OnPairReady func(pairID int64, cleanWord string)

type Business struct {
	cache    *cache.Cache
	dictRepo DictionaryRepository
	aiClient *ai.Client // optional, can be nil
	// atomic: /ai flips it from the admin handler's goroutine while detached
	// storeTranslationPair goroutines read it.
	aiFormattingEnabled atomic.Bool
	log                 *logrus.Logger
	onPairReady         OnPairReady
	pool                wordPool

	cacheHits   atomic.Int64
	cacheMisses atomic.Int64

	// miss collapses concurrent lookups of the same failing query into one
	// cascade. Keyed by the normalized word, same key the cache uses.
	miss singleflight.Group

	// foldedReady goes true once the folded columns are filled. Until then a
	// stored word whose palochka the user dropped reads as absent, and caching
	// that emptiness would pin a wrong answer for the whole negative TTL —
	// finishing the backfill does not go back and invalidate it.
	foldedReady atomic.Bool

	// bg tracks detached persistence work (pair storage, AI formatting, cache
	// writes) so shutdown can wait for it instead of cutting writes mid-flight.
	bg sync.WaitGroup
}

// WaitBackground blocks until detached background work has finished.
func (b *Business) WaitBackground() {
	b.bg.Wait()
}

// TranslationCacheStats returns hit/miss counts for the translation cache
// since process start.
func (b *Business) TranslationCacheStats() (hits, misses int64) {
	return b.cacheHits.Load(), b.cacheMisses.Load()
}

// maxFormHeadwords caps how many lemmas one inflected query may open. A form
// shared by more than a couple of words is a coincidence, not an answer.
const maxFormHeadwords = 3

type DictionaryRepository interface {
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	FindTranslationPairsByFolded(ctx context.Context, folded string, limit int) ([]models.TranslationPairs, error)
	FindHeadwordsByForm(ctx context.Context, folded string, limit int) ([]string, error)
	SaveWordForms(ctx context.Context, headword string, forms []string) error
	FindTranslationPairsByPrefix(ctx context.Context, prefix string, limit int) ([]models.TranslationPairs, error)
	InsertTranslationPair(ctx context.Context, pair repository.TranslationPair) (int64, bool, error)
	UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error
}

func NewBusiness(cache *cache.Cache, dictRepo DictionaryRepository, aiClient *ai.Client, log *logrus.Logger) *Business {
	b := &Business{
		cache:    cache,
		dictRepo: dictRepo,
		aiClient: aiClient,
		log:      log,
	}
	b.aiFormattingEnabled.Store(aiClient != nil)
	return b
}

// SetOnPairReady sets a callback that fires after a pair is saved and AI-formatted.
func (b *Business) SetOnPairReady(fn OnPairReady) {
	b.onPairReady = fn
}

func (b *Business) SetAIFormatting(enabled bool) {
	b.aiFormattingEnabled.Store(enabled)
}

func (b *Business) AIFormattingEnabled() bool {
	return b.aiFormattingEnabled.Load()
}

// Translate returns the ranked pairs for a word, discarding which headword
// answered. Callers that render a card want TranslateResolved instead.
func (b *Business) Translate(word string) ([]models.TranslationPairs, error) {
	pairs, _, err := b.TranslateResolved(word)
	return pairs, err
}

// TranslateResolved is Translate plus the headword the answer belongs to, empty
// when that is the query itself. The card renders against a headword, so an
// answer reached through a different one — «лоьман» resolved to «лом» — renders
// empty and gets thrown away unless the caller knows to re-key it.
//
// An empty result with a nil error is a real "no such word" and is
// negative-cached; a non-nil error means the dictionary could not answer, and
// callers must not read that as absence — it is the difference between telling
// a user their word is missing and telling them the service is down, and
// between recording a genuine vocabulary gap and poisoning missing_words with
// every query made during an outage.
func (b *Business) TranslateResolved(word string) ([]models.TranslationPairs, string, error) {
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	if translations, ok := b.loadCachedTranslations(ctx, cacheKey); ok {
		return translations, "", nil
	}

	if translations := b.loadLocalTranslations(ctx, word); len(translations) > 0 {
		translations = rankAndDedup(translations, word)
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
		return translations, "", nil
	}

	// The stored headword may carry marks no keyboard has — a palochka, a long
	// vowel — that the user simply left out. Matching on the folded columns
	// costs one indexed lookup and no API call.
	if translations := b.loadFoldedTranslations(ctx, word); len(translations) > 0 {
		// Deliberately not cached. The cache key is the user's spelling, and a
		// word has many palochka-less spellings, so moderation's
		// invalidateCacheForPair — which only knows original_clean and
		// translation_clean — could never reach them: a pair a moderator deleted
		// would keep being served under «гала» for the full 30-day TTL. The
		// lookup it replaces is one indexed read, so caching buys almost nothing.
		return rankAndDedup(translations, word), "", nil
	}

	// «лоьман» is «лом» declined, and the grammar card has been printing that
	// paradigm all along without anything indexing it. Not cached, for the same
	// reason the folded layer is not: the key would be a spelling moderation
	// cannot reach.
	if translations, headword := b.loadFormTranslations(ctx, word); len(translations) > 0 {
		return translations, headword, nil
	}

	// A miss is the expensive path: a primary lookup plus a cascade of
	// respellings. Collapsing concurrent misses on the same query means ten
	// people typing one typo cost one cascade rather than ten.
	v, err, _ := b.miss.Do(cacheKey, func() (any, error) {
		return b.fetchTranslationsWithFallback(word)
	})
	if err != nil {
		return nil, "", err
	}
	translations, _ := v.([]models.TranslationPairs)
	translations = rankAndDedup(translations, word)
	if len(translations) > 0 {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
		return translations, "", nil
	}

	// Russian endings have no paradigm to consult, so the lemma is guessed from
	// the stem — but only here, once dosham has said it holds nothing under this
	// spelling. Guessing earlier answers «столб», a word of its own, with «стол».
	// Not cached: the key is the form the user typed, which moderation cannot
	// reach, and the layer is one indexed read anyway.
	if stemmed, headword := b.loadStemTranslations(ctx, word); len(stemmed) > 0 {
		return stemmed, headword, nil
	}

	if b.foldedReady.Load() {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
	}
	return translations, "", nil
}

// SetFoldedReady marks the folded columns as filled, so misses may be
// negative-cached again. Called once, after the startup backfill.
func (b *Business) SetFoldedReady() { b.foldedReady.Store(true) }

// RecheckTranslation queries the API afresh, bypassing the negative cache,
// and reports whether the word has translations now. Used by the daily
// missing-words sweep; a found result is cached so the next search is instant.
func (b *Business) RecheckTranslation(word string) bool {
	// An outage is not evidence the word is still missing, so it stays on the
	// list and gets another chance on the next sweep.
	translations, err := b.fetchTranslationsWithFallback(word)
	if err != nil || len(translations) == 0 {
		return false
	}
	// This path bypasses Translate and writes under the same key, so it has to
	// rank too — otherwise the sweep quietly caches an unranked list.
	b.cacheTranslationsAsync(context.Background(), normalizeCacheKey(word), rankAndDedup(translations, word))
	return true
}

func normalizeCacheKey(word string) string {
	return tools.NormalizeSearch(word)
}

func (b *Business) loadCachedTranslations(ctx context.Context, cacheKey string) ([]models.TranslationPairs, bool) {
	translations, err := b.cache.GetTranslation(ctx, cacheKey)
	if err != nil {
		if !errors.Is(err, cache.ErrMiss) {
			b.log.Printf("cache get failed for %q: %v\n", cacheKey, err)
		}
		b.cacheMisses.Add(1)
		return nil, false
	}
	b.cacheHits.Add(1)
	return translations, true
}

func (b *Business) cacheTranslationsAsync(ctx context.Context, cacheKey string, translations []models.TranslationPairs) {
	b.bg.Go(func() {
		if err := b.cache.SetTranslation(ctx, cacheKey, translations); err != nil {
			b.log.Printf("failed to cache translation: %v\n", err)
		}
	})
}

func (b *Business) loadLocalTranslations(ctx context.Context, word string) []models.TranslationPairs {
	if b.dictRepo == nil {
		return nil
	}
	cleanWord := tools.NormalizeSearch(word)
	if cleanWord == "" {
		return nil
	}
	translations, err := b.dictRepo.FindTranslationPairs(ctx, cleanWord, 200)
	if err != nil {
		b.log.Printf("failed to read dictionary pairs: %v\n", err)
		return nil
	}
	return translations
}

// loadFoldedTranslations retries the local table with the spelling-insensitive
// key: the palochka and the combining marks dropped. It is what makes
// «чегардиг» find «чӏегӏардиг» for any word already stored, at no cost to
// dosham — and it closes the long-vowel gap too, where a stored «лесто̃» used
// to be unreachable by typing «лесто».
func (b *Business) loadFoldedTranslations(ctx context.Context, word string) []models.TranslationPairs {
	if b.dictRepo == nil {
		return nil
	}
	folded := tools.FoldSearch(word)
	if folded == "" {
		return nil
	}
	translations, err := b.dictRepo.FindTranslationPairsByFolded(ctx, folded, 200)
	if err != nil {
		b.log.Printf("failed to read folded dictionary pairs: %v\n", err)
		return nil
	}
	return translations
}

// loadFormTranslations answers an inflected query with its lemma's card. The
// paradigm comes from whatever grammar cards have already been drawn, so the
// table warms itself: looking up «лом» is what makes «лоьман» findable later.
func (b *Business) loadFormTranslations(ctx context.Context, word string) ([]models.TranslationPairs, string) {
	if b.dictRepo == nil {
		return nil, ""
	}
	folded := tools.FoldSearch(word)
	if folded == "" {
		return nil, ""
	}
	headwords, err := b.dictRepo.FindHeadwordsByForm(ctx, folded, maxFormHeadwords)
	if err != nil {
		b.log.Printf("failed to read word forms: %v\n", err)
		return nil, ""
	}

	var out []models.TranslationPairs
	answered := ""
	for _, h := range headwords {
		// Ranked against the headword, not the form the user typed: the card is
		// the lemma's, and rankPair measures distance from its own headword.
		pairs := rankAndDedup(b.loadLocalTranslations(ctx, h), h)
		if len(pairs) > 0 && answered == "" {
			answered = h
		}
		out = append(out, pairs...)
	}
	return out, answered
}

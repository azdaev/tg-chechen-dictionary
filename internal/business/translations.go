package business

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"sort"
	"unicode/utf8"

	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// OnPairReady is called after a new pair is saved and AI formatting completes (or is skipped).
// pairID is the database ID, cleanWord is the normalized search term.
type OnPairReady func(pairID int64, cleanWord string)

type Business struct {
	cache               *cache.Cache
	dictRepo            DictionaryRepository
	aiClient            *ai.Client // optional, can be nil
	aiFormattingEnabled bool
	log                 *logrus.Logger
	onPairReady         OnPairReady
	pool                wordPool

	cacheHits   atomic.Int64
	cacheMisses atomic.Int64

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

type DictionaryRepository interface {
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	FindTranslationPairsByPrefix(ctx context.Context, prefix string, limit int) ([]models.TranslationPairs, error)
	InsertTranslationPair(ctx context.Context, pair repository.TranslationPair) (int64, bool, error)
	UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error
}

func NewBusiness(cache *cache.Cache, dictRepo DictionaryRepository, aiClient *ai.Client, log *logrus.Logger) *Business {
	return &Business{
		cache:               cache,
		dictRepo:            dictRepo,
		aiClient:            aiClient,
		aiFormattingEnabled: aiClient != nil,
		log:                 log,
	}
}

// SetOnPairReady sets a callback that fires after a pair is saved and AI-formatted.
func (b *Business) SetOnPairReady(fn OnPairReady) {
	b.onPairReady = fn
}

func (b *Business) SetAIFormatting(enabled bool) {
	b.aiFormattingEnabled = enabled
}

func (b *Business) AIFormattingEnabled() bool {
	return b.aiFormattingEnabled
}

// Translate returns the ranked pairs for a word. An empty result with a nil
// error is a real "no such word" and is negative-cached; a non-nil error means
// the dictionary could not answer, and callers must not read that as absence —
// it is the difference between telling a user their word is missing and telling
// them the service is down, and between recording a genuine vocabulary gap and
// poisoning missing_words with every query made during an outage.
func (b *Business) Translate(word string) ([]models.TranslationPairs, error) {
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	if translations, ok := b.loadCachedTranslations(ctx, cacheKey); ok {
		return translations, nil
	}

	if translations := b.loadLocalTranslations(ctx, word); len(translations) > 0 {
		translations = rankAndDedup(translations, word)
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
		return translations, nil
	}

	translations, err := b.fetchTranslationsWithFallback(word)
	if err != nil {
		return nil, err
	}
	translations = rankAndDedup(translations, word)
	b.cacheTranslationsAsync(ctx, cacheKey, translations)
	return translations, nil
}

func normalizeText(text string) string {
	return tools.NormalizeSearch(text)
}

func inferOriginalLang(translationLang string) string {
	switch translationLang {
	case "RUS":
		return "CHE"
	case "CHE":
		return "RUS"
	default:
		return ""
	}
}

// normalizeLang maps the dosham API's ISO language codes ("ce"/"ru") to the
// internal representation ("CHE"/"RUS") used throughout storage and display.
// Returns "" for any other language so non-RUS/CHE translations are skipped.
// Both the new ("ce"/"ru") and legacy ("CHE"/"RUS") codes are accepted.
func normalizeLang(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "ce", "che": // Chechen
		return "CHE"
	case "ru", "rus": // Russian
		return "RUS"
	default:
		return ""
	}
}

// doshamAPIURL returns the dosham.app GraphQL endpoint, overridable via the
// DOSHAM_API_URL env var. Defaults to the current production endpoint.
func doshamAPIURL() string {
	if url := strings.TrimSpace(os.Getenv("DOSHAM_API_URL")); url != "" {
		return url
	}
	return "https://api.dosham.app/gql"
}

func toNullString(v string) sql.NullString {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

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

// fetchTranslationsWithFallback queries the API for word and, when the results
// lack an exact headword match for a ё/е-ambiguous query, retries with the
// candidate respellings and puts those results first. The dosham search is a
// substring match that does not fold ё/е — "елка" matches "Белка" but not
// "Ёлка" — while Russians routinely type е for ё. Variants are tried one ё at
// a time ("береза" → "бёреза", "берёза"), stopping at the first exact match.
func (b *Business) fetchTranslationsWithFallback(word string) ([]models.TranslationPairs, error) {
	word = strings.TrimSpace(word)
	translations, err := b.fetchTranslationsFromAPI(word)
	if err == nil && hasExactOriginal(translations, word) {
		return translations, nil
	}

	variants := tools.YoVariants(word)
	if len(variants) == 0 {
		return translations, err
	}

	// The user is already waiting on a miss, so variant lookups run
	// concurrently instead of chaining API round trips. Merging still follows
	// variant order, so the result is the same as the sequential version.
	results := make([][]models.TranslationPairs, len(variants))
	errs := make([]error, len(variants))
	var wg sync.WaitGroup
	for i, alt := range variants {
		wg.Go(func() { results[i], errs[i] = b.fetchTranslationsFromAPI(alt) })
	}
	wg.Wait()

	for i, altPairs := range results {
		if err == nil {
			err = errs[i]
		}
		if len(altPairs) == 0 {
			continue
		}
		translations = mergePairs(altPairs, translations)
		if hasExactOriginal(translations, word) {
			break
		}
	}
	// Something answered, so this is a real result even if a spelling variant
	// failed on the way — at worst we missed an extra respelling. Only a
	// cascade that found nothing AND had a query fail is an outage, and that
	// one must not be negative-cached as "no such word" for a day.
	if len(translations) > 0 {
		return translations, nil
	}
	return translations, err
}

// hasExactOriginal reports whether any pair's headword is exactly the searched
// word (case- and ё/е-insensitive).
func hasExactOriginal(pairs []models.TranslationPairs, word string) bool {
	// Same normalization as ranking. A headword that ranking calls an exact
	// match must not read as a miss here, or the ё-variant fallback fires live
	// queries for a word that was already found.
	key := normalizeForRank(word)
	for _, p := range pairs {
		if normalizeForRank(p.Original) == key {
			return true
		}
	}
	return false
}

// mergePairs returns first followed by second, dropping duplicate pairs.
func mergePairs(first, second []models.TranslationPairs) []models.TranslationPairs {
	merged := make([]models.TranslationPairs, 0, len(first)+len(second))
	seen := make(map[string]bool, len(first)+len(second))
	add := func(pairs []models.TranslationPairs) {
		for _, p := range pairs {
			k := tools.NormalizeSearch(p.Original) + "\x00" + tools.NormalizeSearch(p.Translate)
			if seen[k] {
				continue
			}
			seen[k] = true
			merged = append(merged, p)
		}
	}
	add(first)
	add(second)
	return merged
}

const (
	maxSuggestTrims  = 4
	minSuggestPrefix = 3
	maxSuggestions   = 3
)

// SuggestTranslations rescues a dead-end query by retrying progressively
// shorter prefixes — Russians often type inflected forms ("яблоками") while
// the dictionary stores lemmas ("Яблоко") that a substring search cannot
// match. Only pairs where one side actually starts with the tried prefix are
// returned, so unrelated substring hits don't surface as suggestions.
// Prefixes are looked up concurrently — the user is already waiting on a
// failed search, so this can't afford to chain API round trips — but the
// longest matching prefix still wins.
func (b *Business) SuggestTranslations(word string) []models.TranslationPairs {
	// A phrase the dictionary lacks as a whole is rescued word by word:
	// "красное яблоко" suggests «Красный» and «Яблоко» instead of nothing.
	if words := phraseWords(word); len(words) > 1 {
		return b.suggestFromPhraseWords(words)
	}

	prefixes := prefixCandidates(word)
	if len(prefixes) == 0 {
		return nil
	}

	// The local table reaches lemmas that dosham's substring search can't
	// ("яблоко" from "яблок"), is indexed, and works offline — so it gets the
	// first shot, longest prefix first.
	if b.dictRepo != nil {
		ctx := context.Background()
		for _, prefix := range prefixes {
			local, err := b.dictRepo.FindTranslationPairsByPrefix(ctx, tools.NormalizeSearch(prefix), maxSuggestions)
			if err != nil {
				b.log.Printf("prefix lookup failed for %q: %v\n", prefix, err)
				break
			}
			if len(local) > 0 {
				return local
			}
		}
	}

	results := make([][]models.TranslationPairs, len(prefixes))
	var wg sync.WaitGroup
	for i, prefix := range prefixes {
		wg.Go(func() {
			pairs, err := b.Translate(prefix)
			if err != nil {
				return
			}
			results[i] = filterPrefixMatches(pairs, prefix)
		})
	}
	wg.Wait()

	for _, matches := range results {
		if len(matches) > maxSuggestions {
			matches = matches[:maxSuggestions]
		}
		if len(matches) > 0 {
			return matches
		}
	}
	return nil
}

// phraseWords splits a multi-word query into distinct lookup-worthy words,
// skipping short particles. Returns nil for single-word queries.
func phraseWords(query string) []string {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) < 2 {
		return nil
	}
	seen := make(map[string]bool, len(fields))
	var out []string
	for _, f := range fields {
		if len([]rune(f)) < minSuggestPrefix {
			continue
		}
		key := tools.NormalizeSearch(f)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
		if len(out) == maxSuggestions {
			break
		}
	}
	return out
}

// suggestFromPhraseWords translates each word of a failed phrase concurrently
// and offers the top pair of every word that resolves.
func (b *Business) suggestFromPhraseWords(words []string) []models.TranslationPairs {
	results := make([][]models.TranslationPairs, len(words))
	var wg sync.WaitGroup
	for i, w := range words {
		wg.Go(func() {
			if pairs, err := b.Translate(w); err == nil && len(pairs) > 0 {
				results[i] = pairs[:1]
			}
		})
	}
	wg.Wait()

	var out []models.TranslationPairs
	for _, r := range results {
		out = append(out, r...)
	}
	return out
}

// prefixCandidates returns the query with 1..maxSuggestTrims trailing runes
// removed, longest first. Single words only — trimming a phrase is meaningless.
func prefixCandidates(word string) []string {
	word = strings.TrimSpace(word)
	if strings.ContainsAny(word, " \t\n") {
		return nil
	}
	runes := []rune(word)
	var out []string
	for range maxSuggestTrims {
		if len(runes)-1 < minSuggestPrefix {
			break
		}
		runes = runes[:len(runes)-1]
		out = append(out, string(runes))
	}
	return out
}

func filterPrefixMatches(pairs []models.TranslationPairs, prefix string) []models.TranslationPairs {
	key := tools.NormalizeSearch(prefix)
	var out []models.TranslationPairs
	for _, p := range pairs {
		if strings.HasPrefix(tools.NormalizeSearch(p.Original), key) ||
			strings.HasPrefix(tools.NormalizeSearch(p.Translate), key) {
			out = append(out, p)
		}
	}
	return out
}

func (b *Business) fetchTranslationsFromAPI(word string) ([]models.TranslationPairs, error) {
	query := `
		query Find($inputText: String!) {
			find(inputText: $inputText) {
				entryId
				content
				type
				subtype
				entryIndex
				notes
				rate
				details
				translations {
					translationId
					content
					languageCode
					notes
				}
			}
		}
	`

	var response models.TranslationResponse
	if err := doDoshamQuery(context.Background(), query, map[string]any{"inputText": word}, &response); err != nil {
		return nil, fmt.Errorf("dosham find %q: %w", word, err)
	}

	translations := make([]models.TranslationPairs, 0)

	type pendingPair struct {
		entry       models.Entry
		translation models.Translation
	}
	var toStore []pendingPair

	// The new API returns a flat list of entries. Each entry carries its
	// translations; we keep only Russian/Chechen ones, normalizing the language
	// code to the internal CHE/RUS representation.
	for _, entry := range response.Data.Find {
		for _, translation := range entry.Translations {
			normLang := normalizeLang(translation.LanguageCode)
			if normLang == "" {
				continue
			}
			translation.LanguageCode = normLang

			translationPair := models.TranslationPairs{
				Original:      tools.EscapeUnclosedTags(entry.Content),
				Translate:     tools.EscapeUnclosedTags(translation.Content),
				OriginalLang:  inferOriginalLang(normLang),
				TranslateLang: normLang,
				Rate:          entry.Rate,
				EntryType:     entry.Type,
				Subtype:       entry.Subtype,
				EntryIndex:    entry.EntryIndex,
				Notes:         entry.Notes,
			}
			translations = append(translations, translationPair)

			toStore = append(toStore, pendingPair{entry, translation})
		}
	}

	// Persisting pairs costs a DB lookup each (plus AI formatting for new ones),
	// and a common word carries dozens of them — run detached so those round
	// trips never sit between the user and the answer.
	if len(toStore) > 0 && b.dictRepo != nil {
		b.bg.Go(func() {
			for _, p := range toStore {
				b.storeTranslationPair(p.entry, p.translation)
			}
		})
	}

	return translations, nil
}

// rankAndDedup puts the answer the user actually searched for first and drops
// duplicates that differ only in stress marks. It runs on the way out of
// Translate rather than inside the API fetch: storeTranslationPair persists
// every looked-up word, so once a word is known the local table — not the API —
// is the steady-state path, and a fix applied only to the fetch would leave the
// bug reachable through the other door.
func rankAndDedup(pairs []models.TranslationPairs, query string) []models.TranslationPairs {
	if len(pairs) < 2 {
		return pairs
	}

	key := normalizeForRank(query)
	out := make([]models.TranslationPairs, 0, len(pairs))
	at := make(map[string]int, len(pairs))
	for _, p := range pairs {
		k := normalizeForRank(p.Original) + "\x00" + normalizeForRank(p.Translate)
		if i, ok := at[k]; ok {
			if betterDuplicate(out[i], p) {
				out[i] = p
			}
			continue
		}
		at[k] = len(out)
		out = append(out, p)
	}

	// Stable, and every tiebreaker deterministic: the first result freezes into
	// the cache, and "Ещё" pagination re-ranks on each call, so an unstable
	// order would shuffle pages between presses. On the local path a query's
	// pairs share bucket 0, and rows stored before the rate column share rate 0
	// too — sort.Slice's pdqsort would order those arbitrarily.
	//
	// Dedup has already removed pairs equal on both sides, so this comparator is
	// a total order and nothing of FindTranslationPairs' ORDER BY survives it.
	// That is why the moderation and shortest-gloss preferences are repeated
	// here: leaving them only in SQL would mean the user never sees them.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rankPair(a, key), rankPair(b, key); ra != rb {
			return ra < rb
		}
		if approved(a) != approved(b) {
			return approved(a)
		}
		if a.Rate != b.Rate {
			return a.Rate > b.Rate
		}
		// Nothing below this line may compare the text itself. Within one source
		// dictionary the arrival order IS the lexicographer's sense order, and
		// sorting by gloss length destroyed it: dosham sends куьг as «рука́
		// (кисть), по́дпись, по́черк, го́лос» and the card showed «го́лос» first
		// because it is the shortest string. sort.SliceStable keeps the source
		// order for everything that ties here, which is exactly what is wanted.
		return false
	})

	// dosham's search is a substring match, so a very short query sweeps the
	// dictionary: «ца» returns 252 pairs, and the card's «Ещё (248)» promises a
	// list nobody will page through. The old cap did this before ranking and
	// kept an arbitrary ten; here the ten are the ranked ones.
	if utf8.RuneCountInString(strings.TrimSpace(query)) <= shortQueryRunes && len(out) > shortQueryResults {
		out = out[:shortQueryResults]
	}

	return out
}

const (
	shortQueryRunes   = 3
	shortQueryResults = 10
)

func normalizeForRank(s string) string {
	return stripStressMarks(tools.NormalizeSearch(s))
}

// betterDuplicate reports whether candidate should replace kept when both
// normalize to the same pair. A moderator's rendering wins, then dosham's own
// weight: keeping whichever arrived first would let the API's response order
// decide which survives, and the loser's rate is gone before ranking sees it.
func betterDuplicate(kept, candidate models.TranslationPairs) bool {
	if approved(kept) != approved(candidate) {
		return approved(candidate)
	}
	return candidate.Rate > kept.Rate
}

// approved reports whether a moderator accepted this pair's AI rendering — the
// only human quality signal the dictionary carries. formatPair renders such a
// pair differently, so it should also lead its relevance bucket.
func approved(p models.TranslationPairs) bool {
	return p.FormattedChosen == "ai" && p.FormattedAI != ""
}

func rankPair(p models.TranslationPairs, key string) int {
	original := normalizeForRank(p.Original)
	switch {
	case original == key:
		return 0
	case normalizeForRank(p.Translate) == key:
		return 1
	case strings.HasPrefix(original, key):
		return 2
	}
	return 3
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

func (b *Business) storeTranslationPair(entry models.Entry, translation models.Translation) {
	if b.dictRepo == nil {
		return
	}

	originalLang := inferOriginalLang(translation.LanguageCode)
	if originalLang == "" {
		return
	}

	pair := repository.TranslationPair{
		OriginalRaw:         strings.TrimSpace(entry.Content),
		OriginalClean:       normalizeText(entry.Content),
		OriginalLang:        originalLang,
		TranslationRaw:      strings.TrimSpace(translation.Content),
		TranslationClean:    normalizeText(translation.Content),
		TranslationLang:     translation.LanguageCode,
		Source:              "api",
		SourceEntryID:       toNullString(entry.EntryID),
		SourceTranslationID: toNullString(translation.TranslationID),
		Rate:                entry.Rate,
		EntryType:           entry.Type,
		Subtype:             entry.Subtype,
		EntryIndex:          entry.EntryIndex,
		EntryNotes:          entry.Notes,
	}
	if pair.OriginalClean == "" || pair.TranslationClean == "" {
		return
	}

	pairID, inserted, err := b.dictRepo.InsertTranslationPair(context.Background(), pair)
	if err != nil {
		b.log.Printf("failed to insert dictionary pair: %v\n", err)
		return
	}
	// Duplicates already went through formatting and moderation when first
	// stored; re-running them would burn AI calls and overwrite the result.
	if !inserted || pairID == 0 {
		return
	}

	if b.aiFormattingEnabled && b.aiClient != nil {
		b.bg.Go(func() { b.formatPairWithAI(pairID, pair.OriginalClean, pair.OriginalRaw, pair.TranslationRaw) })
	} else if b.onPairReady != nil {
		// No AI client — trigger moderation immediately
		b.bg.Go(func() { b.onPairReady(pairID, pair.OriginalClean) })
	}
}

// formatPairWithAI asynchronously formats a dictionary pair using AI, saves it, then triggers moderation.
func (b *Business) formatPairWithAI(pairID int64, cleanWord, originalRaw, translationRaw string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build raw entry for formatting
	rawEntry := fmt.Sprintf("**%s** - %s", originalRaw, translationRaw)

	// Format with AI
	formatted, err := b.aiClient.FormatDictionaryEntry(ctx, rawEntry)
	if err != nil {
		b.log.Printf("ai formatting failed for pair %d: %v\n", pairID, err)
	} else {
		// Save to database
		if err := b.dictRepo.UpdateTranslationPairFormatting(ctx, pairID, formatted, ""); err != nil {
			b.log.Printf("failed to save ai formatting for pair %d: %v\n", pairID, err)
		} else {
			b.log.Printf("successfully formatted pair %d with AI\n", pairID)
		}
	}

	// Trigger moderation after AI formatting (or failed attempt)
	if b.onPairReady != nil {
		b.onPairReady(pairID, cleanWord)
	}
}

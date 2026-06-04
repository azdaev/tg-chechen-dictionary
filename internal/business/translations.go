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
}

type DictionaryRepository interface {
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	InsertTranslationPair(ctx context.Context, pair repository.TranslationPair) (int64, error)
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

func (b *Business) Translate(word string) []models.TranslationPairs {
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	if translations, ok := b.loadCachedTranslations(ctx, cacheKey); ok {
		return translations
	}

	if translations := b.loadLocalTranslations(ctx, word); len(translations) > 0 {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
		return translations
	}

	// nil means the API errored and is not cached; an empty non-nil slice is a
	// real "no results" answer and is negative-cached so repeating a dead-end
	// query doesn't redo the whole fallback cascade.
	translations := b.fetchTranslationsWithFallback(word)
	if translations != nil {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
	}

	return translations
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

// TranslateFormatted возвращает переводы с отформатированным текстом, используя кэширование
func (b *Business) TranslateFormatted(word string) *models.TranslationResult {
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	result := b.loadCachedFormatted(ctx, cacheKey)
	if result != nil {
		return result
	}

	// Если в кэше нет, получаем переводы обычным способом
	translations := b.Translate(word)
	if len(translations) == 0 {
		return &models.TranslationResult{
			Pairs:     []models.TranslationPairs{},
			Formatted: "",
		}
	}

	result = &models.TranslationResult{
		Pairs:     translations,
		Formatted: tools.FormatPairs(translations),
	}

	if len(result.Pairs) > 0 {
		b.cacheFormattedAsync(ctx, cacheKey, result)
	}

	return result
}

// fetchTranslationsWithFallback queries the API for word and, when the results
// lack an exact headword match for a ё/е-ambiguous query, retries with the
// candidate respellings and puts those results first. The dosham search is a
// substring match that does not fold ё/е — "елка" matches "Белка" but not
// "Ёлка" — while Russians routinely type е for ё. Variants are tried one ё at
// a time ("береза" → "бёреза", "берёза"), stopping at the first exact match.
func (b *Business) fetchTranslationsWithFallback(word string) []models.TranslationPairs {
	word = strings.TrimSpace(word)
	translations := b.fetchTranslationsFromAPI(word)
	if hasExactOriginal(translations, word) {
		return translations
	}

	for _, alt := range tools.YoVariants(word) {
		altPairs := b.fetchTranslationsFromAPI(alt)
		if len(altPairs) == 0 {
			continue
		}
		translations = mergePairs(altPairs, translations)
		if hasExactOriginal(translations, word) {
			break
		}
	}
	return translations
}

// hasExactOriginal reports whether any pair's headword is exactly the searched
// word (case- and ё/е-insensitive).
func hasExactOriginal(pairs []models.TranslationPairs, word string) bool {
	key := tools.NormalizeSearch(word)
	for _, p := range pairs {
		if tools.NormalizeSearch(p.Original) == key {
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
func (b *Business) SuggestTranslations(word string) []models.TranslationPairs {
	for _, prefix := range prefixCandidates(word) {
		matches := filterPrefixMatches(b.Translate(prefix), prefix)
		if len(matches) > maxSuggestions {
			matches = matches[:maxSuggestions]
		}
		if len(matches) > 0 {
			return matches
		}
	}
	return nil
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

func (b *Business) fetchTranslationsFromAPI(word string) []models.TranslationPairs {
	query := `
		query Find($inputText: String!) {
			find(inputText: $inputText) {
				entryId
				content
				type
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
		b.log.Printf("dosham find query failed: %v\n", err)
		return nil
	}

	translations := make([]models.TranslationPairs, 0)

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
				Original:  tools.EscapeUnclosedTags(entry.Content),
				Translate: tools.EscapeUnclosedTags(translation.Content),
			}
			translations = append(translations, translationPair)

			b.storeTranslationPair(entry, translation)
		}
	}

	if utf8.RuneCountInString(word) <= 3 && len(translations) >= 10 {
		translations = translations[:10]
	}

	// Sort translations by length of the original word (shortest to longest)
	sort.Slice(translations, func(i, j int) bool {
		return utf8.RuneCountInString(translations[i].Original) < utf8.RuneCountInString(translations[j].Original)
	})

	return translations
}

func normalizeCacheKey(word string) string {
	return tools.NormalizeSearch(word)
}

func (b *Business) loadCachedTranslations(ctx context.Context, cacheKey string) ([]models.TranslationPairs, bool) {
	translations, err := b.cache.Get(ctx, cacheKey)
	if err != nil {
		if !errors.Is(err, cache.ErrMiss) {
			b.log.Printf("cache get failed for %q: %v\n", cacheKey, err)
		}
		return nil, false
	}
	return translations, true
}

func (b *Business) cacheTranslationsAsync(ctx context.Context, cacheKey string, translations []models.TranslationPairs) {
	go func() {
		if err := b.cache.Set(ctx, cacheKey, translations); err != nil {
			b.log.Printf("failed to cache translation: %v\n", err)
		}
	}()
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

func (b *Business) loadCachedFormatted(ctx context.Context, cacheKey string) *models.TranslationResult {
	result, err := b.cache.GetTranslationResult(ctx, cacheKey)
	if err != nil {
		if !errors.Is(err, cache.ErrMiss) {
			b.log.Printf("cache get failed for %q: %v\n", "formatted_"+cacheKey, err)
		}
		return nil
	}
	if len(result.Pairs) == 0 {
		return nil
	}
	return result
}

func (b *Business) cacheFormattedAsync(ctx context.Context, cacheKey string, result *models.TranslationResult) {
	if result == nil || len(result.Pairs) == 0 {
		return
	}
	go func() {
		if err := b.cache.SetTranslationResult(ctx, cacheKey, result); err != nil {
			b.log.Printf("failed to cache formatted translation: %v\n", err)
		}
	}()
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
	}
	if pair.OriginalClean == "" || pair.TranslationClean == "" {
		return
	}

	pairID, err := b.dictRepo.InsertTranslationPair(context.Background(), pair)
	if err != nil {
		b.log.Printf("failed to insert dictionary pair: %v\n", err)
		return
	}

	if b.aiFormattingEnabled && b.aiClient != nil && pairID > 0 {
		go b.formatPairWithAI(pairID, pair.OriginalClean, pair.OriginalRaw, pair.TranslationRaw)
	} else if b.onPairReady != nil && pairID > 0 {
		// No AI client — trigger moderation immediately
		go b.onPairReady(pairID, pair.OriginalClean)
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

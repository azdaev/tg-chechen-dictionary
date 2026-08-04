// dosham lookups and the respelling cascade behind a miss: the palochka the
// user could not type, the ъ they dropped, the phrase they typed whole.
package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

// fetchTranslationsWithFallback queries the API for word and, when the results
// lack an exact headword match, retries with the candidate respellings and puts
// those results first. The dosham search is a substring match that folds
// neither ё/е nor the palochka — "елка" matches "Белка" but not "Ёлка", and
// "чегӏардиг" matches nothing at all — while people routinely type е for ё and
// leave out a letter their keyboard does not have. tools.RespellVariants picks
// which defect to retry; the cascade stops at the first exact match.
func (b *Business) fetchTranslationsWithFallback(word string) ([]models.TranslationPairs, error) {
	word = strings.TrimSpace(word)
	translations, err := b.fetchTranslationsFromAPI(word)
	if err == nil && hasExactOriginal(translations, word) {
		return translations, nil
	}

	// A dictionary that returned nothing at all for this spelling is the signal
	// that the query dropped its only palochka, which is precisely the case
	// LooksChechen cannot see.
	variants := tools.RespellVariants(word, err == nil && len(translations) == 0)
	if len(variants) == 0 {
		return translations, err
	}

	// The user is already waiting on a miss, so variant lookups run
	// concurrently instead of chaining API round trips. Merging still follows
	// variant order, so the result is the same as the sequential version.
	// The whole cascade gets one budget. Without it the retry pool turns a burst
	// into a queue: eight callers each holding a handler slot enqueue up to
	// sixty-four retries, and if dosham is timing out at 15s they drain in eight
	// waves — over two minutes during which the bot answers nobody. The pool
	// bounds how much we ask of dosham; this bounds how long we wait for it.
	ctx, cancel := context.WithTimeout(context.Background(), cascadeBudget)
	defer cancel()
	results := make([][]models.TranslationPairs, len(variants))
	errs := make([]error, len(variants))
	var wg sync.WaitGroup
	for i, alt := range variants {
		wg.Go(func() {
			results[i], errs[i] = b.fetchRetryFromAPI(ctx, alt)
			// A hit ends the cascade for real: the rest are cancelled in flight
			// rather than merely ignored, so dosham never serves them.
			if hasFoldedOriginal(results[i], word) {
				cancel()
			}
		})
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
		if hasFoldedOriginal(translations, word) {
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

func (b *Business) fetchTranslationsFromAPI(word string) ([]models.TranslationPairs, error) {
	return b.fetchFromAPI(context.Background(), word)
}

func (b *Business) fetchFromAPI(ctx context.Context, word string) ([]models.TranslationPairs, error) {
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
	if err := doDoshamQuery(ctx, query, map[string]any{"inputText": word}, &response); err != nil {
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

// hasFoldedOriginal reports whether any pair's headword matches the query once
// both are folded. It, not hasExactOriginal, is the cascade's stop signal:
// normalizeForRank keeps the palochka, so «чӏегӏардиг» never reads as an exact
// match for «чегӏардиг» — the one comparison the palochka cascade exists to
// make. Judging respellings by the strict key meant nothing ever stopped the
// cascade and every candidate's substring hits merged into the answer.
func hasFoldedOriginal(pairs []models.TranslationPairs, word string) bool {
	key := tools.FoldSearch(word)
	if key == "" {
		return false
	}
	for _, p := range pairs {
		if tools.FoldSearch(p.Original) == key {
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

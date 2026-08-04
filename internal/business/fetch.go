// dosham lookups and the respelling cascade behind a miss: the palochka the
// user could not type, the ъ they dropped, the phrase they typed whole.
package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// fetchTranslationsWithFallback queries the API for word and, when the results
// lack an exact headword match, retries with the candidate respellings and puts
// those results first. The dosham search folds neither ё/е nor the palochka —
// "елка" matches "Белка" but not "Ёлка", and "чегӏардиг" matches nothing at all
// — while people routinely type е for ё and leave out a letter their keyboard
// does not have. tools.RespellVariants picks which defect to retry; the cascade
// stops at the first exact match.
//
// Guessing the spelling is the only way in, because the obvious shortcut is not
// there. find() reads like a substring search — it answers «дом» with «Домбра»
// — but it is fuzzier and word-oriented than that, and a fragment of a word does
// not reliably reach it: find("ард") returns thirteen entries and «чӀе̃гӀардиг»
// is not among them, though it contains those letters, while find("ардиг")
// returns nineteen and it is. find("гумки") returns nothing at all for
// «гӏазгӏумки». So a query that dropped every palochka cannot be recovered by
// searching what survived between them — measured on eleven live two-palochka
// words, two were reachable that way. The layer that does cover them is local:
// the folded columns match any spelling of a word already stored, which is
// every word anyone has looked up.
func (b *Business) fetchTranslationsWithFallback(word string) ([]models.TranslationPairs, error) {
	word = strings.TrimSpace(word)
	translations, err := b.fetchTranslationsFromAPI(word)
	if err == nil && hasExactOriginal(translations, word) {
		return translations, nil
	}

	// Being told to slow down is the one answer that must not be met with four
	// more requests: the cascade would turn one refused lookup into five, at the
	// moment the dictionary can least afford it.
	//
	// ponytail: per-lookup only. A burst still costs one refused request each;
	// a shared breaker that skips the API outright for a few seconds after a 429
	// is the next step if this ever shows up in the logs as a wave.
	if errors.Is(err, errRateLimited) {
		return translations, err
	}

	// A dictionary that returned nothing at all for this spelling is the signal
	// that the query dropped its only palochka, which is precisely the case
	// LooksChechen cannot see.
	variants := tools.RespellVariants(word, err == nil && len(translations) == 0)
	if len(variants) == 0 {
		return translations, err
	}

	// The user is already waiting on a miss, so variant lookups run
	// concurrently instead of chaining API round trips.
	//
	// Merging follows variant order, but the cancellation below does not: a
	// candidate that answers first ends the ones still queued behind the retry
	// pool, whichever order RespellVariants put them in. So this is not the
	// sequential cascade with the waiting removed — when two respellings are
	// both real words, which one answers depends on timing, and the winner is
	// what gets cached. Left as is deliberately: every candidate that could win
	// folds to the query, so all of them are the word the user typed under a
	// spelling they did not, and preserving strict priority would mean spending
	// the queued calls on a volunteer API to choose between right answers.
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
	// failed on the way — at worst we missed an extra respelling.
	//
	// Rows are not an answer, though. dosham returns substring noise for almost
	// any query, and the card refuses it: «стрим» comes back with «гольфстрим»
	// and renders nothing. Reporting that as success while a variant lookup was
	// failing caches the noise for the full TTL, so the one spelling that would
	// have answered stays unreachable long after the API recovers — and the user
	// is told the word does not exist. The card is asked because it is the thing
	// that decides, and only here, on the path that already failed.
	if len(translations) > 0 && (err == nil || tools.Render(word, translations).Body != "") {
		return translations, nil
	}
	return translations, err
}

// maxAIFormatsPerLookup is how many newly stored pairs one lookup may send to
// the LLM for a moderation suggestion.
const maxAIFormatsPerLookup = 10

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
				Packed:        normLang == "CHE",
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
	//
	// Every pair is stored: the local table is what makes the next lookup of any
	// of them instant, and what the folded columns match a palochka-less
	// spelling against. The LLM rendering is budgeted instead. dosham answers
	// «ца» with 252 pairs and each new one used to start its own goroutine and
	// its own paid call — for a card that shows ten rows. The rendering only
	// feeds the moderation queue, and a queue nobody can read to the end of is
	// not worth what it costs.
	if len(toStore) > 0 && b.dictRepo != nil {
		b.bg.Go(func() {
			budget := maxAIFormatsPerLookup
			for _, p := range toStore {
				if b.storeTranslationPair(p.entry, p.translation, budget > 0) && budget > 0 {
					budget--
				}
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

// mergePairs returns first followed by second, dropping duplicates. It shares
// ranking's definition of a duplicate and of which duplicate to keep, so a
// respelling that answered first cannot bury a better-sourced row here before
// ranking gets to choose between them.
func mergePairs(first, second []models.TranslationPairs) []models.TranslationPairs {
	return dedupPairs(first, second)
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

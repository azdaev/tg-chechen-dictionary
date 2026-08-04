// What to offer when nothing matched: prefixes of the query and the words of a
// phrase that has no entry of its own.
package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"strings"
	"sync"
)

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

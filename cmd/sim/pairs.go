package main

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"fmt"
	"unicode/utf8"
)

// The two steps between dosham's answer and the renderer, kept in the same
// order the bot performs them. Anything the simulator does differently here is
// a lie about what a user would see.

// toPairs flattens dosham entries into the pairs the renderer consumes. Mirrors
// business.pairsFromEntries. Entries with no recognised language are dropped,
// which is why an entry can contribute nothing.
func toPairs(entries []entry, verbose bool) []models.TranslationPairs {
	var pairs []models.TranslationPairs
	for _, e := range entries {
		for _, t := range e.Translations {
			lang := normLang(t.LanguageCode)
			if lang == "" {
				continue
			}
			if verbose {
				fmt.Printf("[%s rate=%d st=%d idx=%d forms=%d rel=%d] %s :: %s\n",
					e.Type, e.Rate, e.Subtype, e.EntryIndex,
					len(e.EntryForms), len(e.RelatedEntries), e.Content, t.Content)
			}
			pairs = append(pairs, models.TranslationPairs{
				Original:      tools.EscapeUnclosedTags(e.Content),
				Translate:     tools.EscapeUnclosedTags(t.Content),
				OriginalLang:  map[string]string{"CHE": "RUS", "RUS": "CHE"}[lang],
				TranslateLang: lang,
				// Packed says the Chechen side is a whole article rather than a
				// gloss, and cannot be inferred from the direction alone.
				Packed:     lang == "CHE",
				Rate:       e.Rate,
				EntryType:  e.Type,
				Subtype:    e.Subtype,
				EntryIndex: e.EntryIndex,
				Notes:      e.Notes,
			})
		}
	}
	return pairs
}

// capShortQuery mirrors business.capShortQuery: a two-letter query matches half
// the dictionary, so entries are capped while the examples that illustrate the
// query itself are counted apart and kept.
func capShortQuery(word string, pairs []models.TranslationPairs) []models.TranslationPairs {
	const shortQueryRunes, maxEntries = 3, 10
	if utf8.RuneCountInString(word) > shortQueryRunes || len(pairs) <= maxEntries {
		return pairs
	}
	folded := tools.FoldSearch(word)
	kept, entries := pairs[:0:0], 0
	for _, p := range pairs {
		if p.EntryType != "TEXT" ||
			tools.FoldSearch(p.Original) == folded || tools.FoldSearch(p.Translate) == folded {
			if entries >= maxEntries {
				continue
			}
			entries++
		}
		kept = append(kept, p)
	}
	return kept
}

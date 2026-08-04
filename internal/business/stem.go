// Answering an inflected query the dictionary only stores as a lemma.
package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"strings"
	"unicode/utf8"
)

// maxLemmaOvershoot is how much longer than the stem a lemma may be. «руки»
// stems to «рук», and «рука» is one letter past it; «рукавица» is six and a
// different word.
const maxLemmaOvershoot = 3

// loadStemTranslations answers an inflected query with its lemma's card:
// «карандаша» → «карандаш», «руки» → «рука», «къоламаш» → «къолам». The word
// forms layer above it only knows paradigms dosham has analyzed, which is
// Chechen only — a Russian ending had no layer at all, so «руки» reached the
// user as «нет перевода» and went into the missing-words report as a gap the
// dictionary does not have.
//
// The stem is the query minus its last letters; the lemma is the shortest
// stored headword that starts with it. Shortest, not the longest stem's first
// hit: «домов» stems to both «домо» and «дом», and preferring the longer stem
// answers with «домовой».
func (b *Business) loadStemTranslations(ctx context.Context, word string) ([]models.TranslationPairs, string) {
	if b.dictRepo == nil {
		return nil, ""
	}

	lemma := ""
	for _, stem := range prefixCandidates(word) {
		key := tools.NormalizeSearch(stem)
		pairs, err := b.dictRepo.FindTranslationPairsByPrefix(ctx, key, maxSuggestions)
		if err != nil {
			b.log.Printf("stem lookup failed for %q: %v\n", stem, err)
			return nil, ""
		}
		for _, p := range pairs {
			// FindTranslationPairsByPrefix leads with the side that matched, so
			// Original is the candidate. A phrase is never a lemma of one word.
			head := tools.NormalizeSearch(p.Original)
			if head == "" || strings.ContainsAny(head, " \t") {
				continue
			}
			if utf8.RuneCountInString(head) > utf8.RuneCountInString(key)+maxLemmaOvershoot {
				continue
			}
			if lemma == "" || utf8.RuneCountInString(head) < utf8.RuneCountInString(lemma) {
				lemma = head
			}
		}
	}
	if lemma == "" || lemma == tools.NormalizeSearch(word) {
		return nil, ""
	}

	// Ranked against the lemma, not the form the user typed: the card is the
	// lemma's, and rankPair measures distance from its own headword.
	pairs := rankAndDedup(b.loadLocalTranslations(ctx, lemma), lemma)
	if len(pairs) == 0 {
		return nil, ""
	}
	return pairs, lemma
}

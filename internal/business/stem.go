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

// maxStemTrims is how much of the query the lemma search may cut. It is the
// stem layer's own budget, deliberately not the suggestion list's: a suggestion
// is offered as a guess and can afford to reach further, while this is served
// as the answer. Sharing one constant meant a UX tweak to «возможно, вы искали»
// would silently change which word the bot states as fact.
const maxStemTrims = 3

// sameStem reports whether two words differ only in their endings — the
// relation a form has to its lemma, and the one the prefix search does not
// check on its own. prefixCandidates trims up to four letters, which is right
// for the «возможно, вы искали» list where a guess is labelled a guess; here
// the result is served as the answer, and «галгайн» trimmed to «гал» came back
// as «галоп» — a Russian card for a Chechen word, with nothing to say it had
// been guessed at.
func sameStem(typed, head string) bool {
	a, b := []rune(typed), []rune(head)
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n+maxLemmaOvershoot >= len(a) && n+maxLemmaOvershoot >= len(b)
}

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
//
// Runs last, after dosham has said it holds nothing under the typed spelling —
// a guess must never pre-empt the dictionary. Placed before it, this layer
// answered «столб» with «стол».
func (b *Business) loadStemTranslations(ctx context.Context, word string) ([]models.TranslationPairs, string) {
	if b.dictRepo == nil {
		return nil, ""
	}

	typed := tools.NormalizeSearch(word)
	lemma := ""
	for _, stem := range prefixCandidates(word, maxStemTrims) {
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
			if !sameStem(typed, head) {
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
	found, err := b.loadLocalTranslations(ctx, lemma)
	if err != nil {
		return nil, ""
	}
	pairs := rankAndDedup(found, lemma)
	if len(pairs) == 0 {
		return nil, ""
	}
	return pairs, lemma
}

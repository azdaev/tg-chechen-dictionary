// Entries that point at another entry instead of translating.
package tools

import (
	"regexp"
	"strings"

	"chetoru/internal/models"
)

var crossRefOnlyRe = regexp.MustCompile(`^см\.\s*([^\s,;.]+)\.?$`)

// CrossRef reports the entry a gloss points at instead of translating.
//
// Chechen marks noun class on the verb, so «ваха», «яха», «баха» and «даха» are
// one word in four shapes. The dictionary spells out the д-form and sends the
// rest to it with «см. даха» — 21 of 545 sampled Chechen entries, among them
// the most ordinary verbs there are. Every one of them reached the reader as a
// card with no translation on it and nothing to click.
//
// Only entries headed by the query itself are read: an example that merely
// contains the word says nothing about where the word is defined, and a
// neighbouring phrase that happens to be translated is not this word's
// meaning. If any of them does translate the query, there is nothing to
// follow — the card already says what it means.
func CrossRef(query string, pairs []models.TranslationPairs) string {
	q := FoldSearch(NormalizeSearch(query))
	target := ""
	for _, p := range pairs {
		gloss := ""
		switch {
		case FoldSearch(NormalizeSearch(p.Original)) == q:
			gloss = p.Translate
		case FoldSearch(NormalizeSearch(p.Translate)) == q:
			gloss = p.Original
		default:
			continue
		}
		m := crossRefOnlyRe.FindStringSubmatch(strings.TrimSpace(Clean(gloss)))
		switch {
		case m == nil:
			return "" // the word is translated here after all
		case target == "":
			target = m[1]
		case NormalizeSearch(target) != NormalizeSearch(m[1]):
			// Two homonyms pointing at two different words. Following one of
			// them would answer half the question and hide the other half.
			return ""
		}
	}
	return target
}

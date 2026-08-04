// Entries that point at another entry instead of translating.
package tools

import (
	"regexp"
	"strings"

	"chetoru/internal/models"
)

var (
	crossRefOnlyRe = regexp.MustCompile(`^см\.\s*([^\s,;.¹²³]+)[¹²³,\s]*\.?$`)
	// The derivational pointers: «яхчийта — понуд. от яхча», «диттадала —
	// потенц. от дитта», «бежан — прил. к бажа». Together with «см.» they are
	// 14% of the Chechen glosses sampled. Unlike «см.» these do not mean the
	// same word, so the base word is shown beside the entry rather than
	// instead of it — a causative is not its own verb.
	derivedFromRe = regexp.MustCompile(`^(?:понуд|потенц|масд|прич|прил|нареч|сущ|уменьш|увелич|многокр|однокр)\.\s*(?:от|к)\s+([^\s,;.¹²³]+)[¹²³,\s]*\.?$`)
)

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
	return pointsAt(query, pairs, crossRefOnlyRe)
}

// DerivedFrom is CrossRef for the pointers that name a base word rather than a
// synonym: «понуд. от яхча» is the causative of яхча, not яхча. The caller
// shows the base word's meaning next to the entry; putting it in place of the
// entry would state that «яхчийта» means «охладить», which it does not.
func DerivedFrom(query string, pairs []models.TranslationPairs) string {
	return pointsAt(query, pairs, derivedFromRe)
}

func pointsAt(query string, pairs []models.TranslationPairs, re *regexp.Regexp) string {
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
		m := re.FindStringSubmatch(strings.TrimSpace(Clean(gloss)))
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

// FirstGloss states what a card says the query means, in one line — the
// leading sense of the block the card leads with.
func FirstGloss(query string, pairs []models.TranslationPairs) string {
	for _, b := range collect(query, pairs).blocks {
		for _, sense := range b.senses {
			for _, variant := range strings.Split(stripParens(sense), ",") {
				if g := withoutLabels(variant); g != "" {
					return g
				}
			}
		}
	}
	return ""
}

// withoutLabels drops the abbreviations a sense opens with. The academic corpus
// writes «прям., перен. закали́ться»; quoting «прям.» as what a word means is
// worse than saying nothing.
func withoutLabels(variant string) string {
	variant = strings.TrimSpace(variant)
	for {
		fields := strings.Fields(variant)
		if len(fields) < 2 || !strings.HasSuffix(fields[0], ".") {
			break
		}
		variant = strings.TrimSpace(strings.TrimPrefix(variant, fields[0]))
	}
	if strings.HasSuffix(variant, ".") {
		return "" // a label standing alone
	}
	return variant
}

// pointersLast moves the senses that only name another entry behind the ones
// that translate. A card whose meanings are «1. понуд. от тоа 2. прекрати́ть»
// spends its first line on a word the reader did not ask about and did not get
// translated either. Moved rather than dropped: the derivation is worth
// knowing, just not first. When every sense is a pointer there is nothing to
// move it behind, and the card stays a pointer — which is what CrossRef and
// DerivedFrom then answer.
func pointersLast(senses []string) []string {
	var plain, pointers []string
	for _, s := range senses {
		if isPointer(s) {
			pointers = append(pointers, s)
			continue
		}
		plain = append(plain, s)
	}
	if len(plain) == 0 {
		return senses
	}
	return append(plain, pointers...)
}

func isPointer(sense string) bool {
	sense = strings.TrimSpace(sense)
	return crossRefOnlyRe.MatchString(sense) || derivedFromRe.MatchString(sense)
}

// Ranking and deduplication of the pairs a lookup returned, whichever layer
// produced them.
package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	shortQueryRunes   = 3
	shortQueryResults = 10
	// Usage examples get a budget of their own. The card renders six lines of
	// them at most, and the whole-word filter and dedup thin the candidates
	// hard, so this is roughly what six lines costs.
	shortQueryExamples = 24
)

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
	out := dedupPairs(pairs)

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

	if utf8.RuneCountInString(strings.TrimSpace(query)) <= shortQueryRunes && len(out) > shortQueryResults {
		out = capShortQuery(out, query)
	}

	return out
}

// capShortQuery trims the sweep a very short query brings back. dosham's search
// is a substring match, so «ца» returns 252 pairs and a card built from all of
// them is a wall of unrelated words. The cap runs after ranking, so what
// survives is the top of the list rather than whatever the API sent first.
//
// Usage examples are counted apart. An example costs the card one line, not a
// block, and it is the thing hardest to do without in Chechen — yet it ranks
// last, so a flat cap cut examples first: «цӀа» kept four glosses and lost all
// six of «цӀа кха̃ча — прибы́ть домо́й» that dosham holds for it. Every short word
// in the language is a basic one, so this hit «цӀе», «ког», «дог», «хи».
func capShortQuery(pairs []models.TranslationPairs, query string) []models.TranslationPairs {
	folded := tools.FoldSearch(query)
	out := make([]models.TranslationPairs, 0, shortQueryResults+shortQueryExamples)
	entries, examples := 0, 0
	for _, p := range pairs {
		if isUsageExample(p, folded) {
			if examples >= shortQueryExamples {
				continue
			}
			examples++
		} else {
			if entries >= shortQueryResults {
				continue
			}
			entries++
		}
		out = append(out, p)
	}
	return out
}

// isUsageExample mirrors the card's own reading: a collocation the user did not
// ask for by name illustrates some entry, it is not an entry itself.
func isUsageExample(p models.TranslationPairs, folded string) bool {
	return p.EntryType == "TEXT" &&
		tools.FoldSearch(p.Original) != folded &&
		tools.FoldSearch(p.Translate) != folded
}

func normalizeForRank(s string) string {
	return stripStressMarks(tools.NormalizeSearch(s))
}

// pairIdentity is what makes two rows the same translation: both sides equal
// once spelling is normalized and stress marks — which only the academic corpus
// writes — are dropped. One definition, because there used to be two: the
// cascade merged on the strict key and kept whichever variant answered first,
// then ranking merged on this one and kept the better row. The first pass could
// therefore throw away the moderated or better-sourced row before the second
// pass was ever asked which to keep.
func pairIdentity(p models.TranslationPairs) string {
	return normalizeForRank(p.Original) + "\x00" + normalizeForRank(p.Translate)
}

// dedupPairs keeps one row per identity — the best one — at the position where
// that identity first appeared.
func dedupPairs(groups ...[]models.TranslationPairs) []models.TranslationPairs {
	total := 0
	for _, g := range groups {
		total += len(g)
	}
	out := make([]models.TranslationPairs, 0, total)
	at := make(map[string]int, total)
	for _, g := range groups {
		for _, p := range g {
			k := pairIdentity(p)
			if i, ok := at[k]; ok {
				if betterDuplicate(out[i], p) {
					out[i] = p
				}
				continue
			}
			at[k] = len(out)
			out = append(out, p)
		}
	}
	return out
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
	// A folded match is the answer to a query that dropped the palochka: the
	// user typed «чегардиг» and meant «чӏегӏардиг». Without this bucket every
	// such hit tied at the bottom with unrelated substring matches, so the word
	// they were actually looking for did not lead its own card. It ranks below
	// an exact hit and above a prefix guess.
	case tools.FoldSearch(original) == tools.FoldSearch(key):
		return 2
	case strings.HasPrefix(original, key):
		return 3
	}
	return 4
}

// Deciding what one dictionary pair is for the query at hand: the query's own
// entry, an example of it, a neighbour of it, or nothing. Rendering lives in
// card.go and starts where this ends.
package tools

import (
	"chetoru/internal/models"
	"strings"
)

// lookup is the query a card is being built for, in the three forms matching
// needs: as typed, normalized, and folded.
type lookup struct {
	raw    string
	key    string
	folded string
}

func newLookup(query string) lookup {
	return lookup{raw: query, key: NormalizeSearch(query), folded: foldPhrase(query)}
}

// role is what a pair turns out to be for the query at hand.
type role int

const (
	roleNone      role = iota // mentions the query nowhere that counts
	roleEntry                 // the query's own entry: a headword and its senses
	roleExample               // illustrates the query while belonging to another entry
	roleNeighbour             // merely starts the same way
)

// placement is the whole decision about one pair, separated from the act of
// filing it. Deciding and filing used to be one switch, which is why every new
// corpus quirk — a punctuation mark, a homonym number — was another case
// wedged between the two.
type placement struct {
	role     role
	head     string
	cheHead  bool
	senses   []string
	examples []example
}

// classify decides what a pair is for this query. It reads dosham's own fields
// — direction, entry type, index — rather than scanning the text for "1)" and
// "~" the way the old renderer did.
func (q lookup) classify(p models.TranslationPairs) placement {
	original, translate := Clean(p.Original), Clean(p.Translate)
	// The Russian–Chechen article is the one corpus that packs a whole entry
	// into a single string, so it is the only place left that parses text.
	if p.TranslateLang == "CHE" {
		return q.classifyArticle(p, original, translate)
	}
	return q.classifyEntry(p, original, translate)
}

func (q lookup) classifyArticle(p models.TranslationPairs, original, translate string) placement {
	glosses, examples := articleParts(p, original, translate)
	switch {
	// The user typed the article's own Russian headword.
	case NormalizeSearch(original) == q.key:
		return placement{role: roleEntry, head: original, senses: glosses, examples: examples}

	// The user typed one of its Chechen glosses, so the answer is the article's
	// headword — this carries «карандаш» onto «къолам».
	case matchingGloss(glosses, q.key) != "":
		return placement{
			role:     roleEntry,
			head:     matchingGloss(glosses, q.key),
			cheHead:  true,
			senses:   []string{strings.ToLower(original)},
			examples: relevant(examples, q.key),
		}

	// Body mention only. Never a sense — «А», «Его» and «Нет» all mention
	// «карандаш» — but its examples for the word are real.
	case containsWord(translate, q.key):
		return placement{role: roleExample, examples: relevant(examples, q.key)}

	case strings.HasPrefix(NormalizeSearch(original), q.key):
		return placement{role: roleNeighbour, head: original}
	}
	return placement{role: roleNone}
}

func (q lookup) classifyEntry(p models.TranslationPairs, original, translate string) placement {
	switch {
	// A collocation: dosham's own usage example, already in two languages.
	// Asked for by name it is an entry — «телефон болх беш яц» is a phrase the
	// dictionary holds, and rendering it only as somebody else's example left
	// the query with no card at all.
	case p.EntryType == "TEXT" && foldPhrase(original) == q.folded:
		return placement{role: roleEntry, head: original, cheHead: p.OriginalLang == "CHE", senses: []string{translate}}

	case p.EntryType == "TEXT" && foldPhrase(translate) == q.folded:
		return placement{role: roleEntry, head: translate, cheHead: p.TranslateLang == "CHE", senses: []string{original}}

	case p.EntryType == "TEXT":
		if containsWord(original, q.key) || containsWord(translate, q.key) {
			return placement{role: roleExample, examples: []example{orient(p, original, translate)}}
		}

	// The query is this entry's headword: its glosses are the answer.
	case NormalizeSearch(original) == q.key:
		return placement{role: roleEntry, head: original, cheHead: p.OriginalLang == "CHE", senses: []string{translate}}

	// The query is one of this entry's glosses: the headword is the answer.
	case NormalizeSearch(translate) == q.key:
		return placement{role: roleEntry, head: translate, cheHead: p.TranslateLang == "CHE", senses: []string{original}}

	// Same, once the marks a keyboard cannot type are folded away. Every layer
	// that reaches this renderer — the folded columns, the palochka cascade,
	// rankPair's own folded bucket — matches on the folded key, so matching only
	// the strict one here threw those answers away and the user was told the
	// word does not exist. Exact stays above, so a true headword still wins.
	case foldPhrase(original) == q.folded:
		return placement{role: roleEntry, head: original, cheHead: p.OriginalLang == "CHE", senses: []string{translate}}

	case foldPhrase(translate) == q.folded:
		return placement{role: roleEntry, head: translate, cheHead: p.TranslateLang == "CHE", senses: []string{original}}

	// Neighbour: how dosham answers «дом» with «Домбра». Never a card, worth
	// one line at the foot.
	case strings.HasPrefix(NormalizeSearch(original), q.key):
		return placement{role: roleNeighbour, head: original}
	}
	return placement{role: roleNone}
}

// matchingGloss finds the article gloss holding the queried word. Glosses list
// variants ("цӏа, цӏехьа"), so the test is whole-word, not equality.
func matchingGloss(glosses []string, key string) string {
	for _, g := range glosses {
		if containsWord(g, key) {
			return g
		}
	}
	return ""
}

// relevant keeps the examples that actually illustrate the queried word.
func relevant(examples []example, key string) []example {
	out := make([]example, 0, len(examples))
	for _, ex := range examples {
		if containsWord(ex.chechen, key) || containsWord(ex.russian, key) {
			out = append(out, ex)
		}
	}
	return out
}

// orient puts the Chechen side first, the order every example in the bot uses.
func orient(p models.TranslationPairs, original, translate string) example {
	if p.TranslateLang == "CHE" {
		return example{chechen: translate, russian: original}
	}
	return example{chechen: original, russian: translate}
}

// containsWord tests for key as a whole word. Substring matching is what makes
// dosham answer «къолам» with the article for «А».
func containsWord(text, key string) bool {
	if key == "" {
		return false
	}
	hay := NormalizeSearch(text)
	for i := 0; ; {
		j := strings.Index(hay[i:], key)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(key)
		if !isWordByte(hay, start-1) && !isWordByte(hay, end) {
			return true
		}
		i = start + len(key)
		if i >= len(hay) {
			return false
		}
	}
}

// isWordByte reports whether the byte at i continues a word. Cyrillic is
// two-byte, so any high byte counts as inside one.
func isWordByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c >= 0x80 || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

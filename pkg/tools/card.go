package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
)

// One shape for every lookup, both directions, all four source dictionaries:
//
//	<b>заголовок</b> · пометы
//	1. смысл
//
//	<i>чеченский пример → русский</i>
//	рядом: соседние слова
//
// Bold marks Chechen — on the header for a Chechen lookup, on the senses for a
// Russian one. Nothing else moves.

// posLabels covers only the subtypes whose `details` key set proves the
// reading; an unlisted one renders no chip, since a wrong part of speech
// miseducates and the chip is decoration.
var posLabels = map[int]string{
	1: "гл.",
	2: "сущ.",
	3: "нареч.",
	4: "прил.",
	6: "мест.",
}

const (
	// Generous by design: these stop a runaway entry («Идти» has 26 senses).
	maxCardSenses       = 10
	maxCardExampleLines = 6
	maxNeighbours       = 12
)

// FormatCard renders one lookup as a single card.
func FormatCard(query string, pairs []models.TranslationPairs) string {
	c := collect(query, pairs)
	if len(c.blocks) == 0 && len(c.neighbours) == 0 {
		return ""
	}
	return c.render()
}

// block is one headword-and-homonym: the unit a card repeats.
type block struct {
	head     string
	cheHead  bool // the header is the Chechen side, so bold goes there
	pos      int
	notes    string
	senses   []string
	examples []example
	index    int // homonym number; 1 when the word has no homonyms
	rate     int // best source dictionary seen for this block
}

type example struct{ chechen, russian string }

type collected struct {
	blocks     []*block
	neighbours []string
}

// collect turns ranked pairs into blocks. Which door a pair takes — headword,
// gloss, article, collocation — is decided by dosham's own fields, not by
// scanning the text for "1)" and "~" the way the old renderer did.
func collect(query string, pairs []models.TranslationPairs) collected {
	key := NormalizeSearch(query)
	var c collected
	blocks := map[string]*block{}

	// EntryIndex 0 means "no homonym number recorded", not "homonym zero", so
	// it folds into 1; otherwise one word becomes two cards.
	blockFor := func(p models.TranslationPairs, head string, cheHead bool) *block {
		idx := p.EntryIndex
		if idx < 1 {
			idx = 1
		}
		k := fmt.Sprintf("%s\x00%d", NormalizeSearch(head), idx)
		b, ok := blocks[k]
		if !ok {
			b = &block{head: head, cheHead: cheHead, index: idx}
			blocks[k] = b
			c.blocks = append(c.blocks, b)
		}
		// The academic corpus owns the spelling — stress marks, palochka.
		if p.Rate > b.rate && head != "" {
			b.head, b.rate = head, p.Rate
		}
		if b.pos == 0 {
			b.pos = p.Subtype
		}
		if b.notes == "" {
			b.notes = p.Notes
		}
		return b
	}

	for _, p := range pairs {
		original, translate := Clean(p.Original), Clean(p.Translate)
		isArticle := p.TranslateLang == "CHE"

		// The Russian–Chechen article: the one corpus that packs a whole entry
		// into one string, so the only place left that parses text.
		if isArticle {
			glosses, examples := articleParts(p, original, translate)
			switch {
			// The user typed the article's own Russian headword.
			case NormalizeSearch(original) == key:
				b := blockFor(p, original, false)
				b.senses = append(b.senses, glosses...)
				b.examples = append(b.examples, examples...)

			// The user typed one of its Chechen glosses, so the answer is the
			// article's headword — this carries «карандаш» onto «къолам».
			case matchingGloss(glosses, key) != "":
				b := blockFor(p, matchingGloss(glosses, key), true)
				b.senses = append(b.senses, strings.ToLower(original))
				b.examples = append(b.examples, relevant(examples, key)...)

			// Body mention only. Never a sense — «А», «Его» and «Нет» all
			// mention «карандаш» — but its examples for the word are real.
			case containsWord(translate, key):
				b := blockFor(p, "", false)
				b.examples = append(b.examples, relevant(examples, key)...)

			case strings.HasPrefix(NormalizeSearch(original), key):
				c.neighbours = append(c.neighbours, original)
			}
			continue
		}

		switch {
		// A collocation: dosham's own usage example, already in two languages.
		case p.EntryType == "TEXT":
			if containsWord(original, key) || containsWord(translate, key) {
				b := blockFor(p, "", false)
				b.examples = append(b.examples, orient(p, original, translate))
			}

		// The query is this entry's headword: its glosses are the answer.
		case NormalizeSearch(original) == key:
			b := blockFor(p, original, p.OriginalLang == "CHE")
			b.senses = append(b.senses, translate)

		// The query is one of this entry's glosses: the headword is the answer.
		case NormalizeSearch(translate) == key:
			b := blockFor(p, translate, p.TranslateLang == "CHE")
			b.senses = append(b.senses, original)

		// Neighbour: how dosham's substring search answers «дом» with «Домбра».
		// Never a card, worth one line at the foot.
		case p.EntryType != "TEXT" && strings.HasPrefix(NormalizeSearch(original), key):
			c.neighbours = append(c.neighbours, original)
		}
	}

	// Example-only blocks were separate just because dosham filed them under
	// another entry; fold them in so the card stays one shape.
	var kept []*block
	var orphaned []example
	for _, b := range c.blocks {
		if len(b.senses) == 0 {
			orphaned = append(orphaned, b.examples...)
			continue
		}
		b.senses = dedupSenses(b.senses)
		kept = append(kept, b)
	}
	if len(kept) > 0 {
		kept[0].examples = append(kept[0].examples, orphaned...)
	}
	for _, b := range kept {
		b.examples = dedupExamples(b.examples, NormalizeSearch(b.head))
	}
	c.blocks = kept
	c.neighbours = dedupStrings(c.neighbours)
	return c
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

func (c collected) render() string {
	var out []string
	for _, b := range c.blocks {
		out = append(out, b.render())
	}
	if len(c.neighbours) > 0 {
		names := c.neighbours
		if len(names) > maxNeighbours {
			names = names[:maxNeighbours]
		}
		out = append(out, "<i>рядом:</i> "+strings.Join(names, ", "))
	}
	return strings.TrimSpace(strings.Join(out, "\n\n"))
}

func (b *block) render() string {
	bold := func(s string) string { return "<b>" + s + "</b>" }
	var lines []string

	head := b.head
	if b.cheHead {
		head = bold(head)
	}
	if b.index > 1 {
		head += superscript(b.index)
	}
	if chip := b.chip(); chip != "" {
		head += " · <i>" + chip + "</i>"
	}
	lines = append(lines, head)

	// Russian qualifiers — "(почерк) хатӏ" — trail the gloss rather than sit
	// inside its bold, since bold marks Chechen and nothing else.
	gloss := func(s string) string {
		quals, rest := splitQualifiers(s)
		if !b.cheHead {
			rest = bold(rest)
		}
		if len(quals) > 0 {
			rest += " <i>(" + strings.Join(quals, ", ") + ")</i>"
		}
		return rest
	}
	senses := b.senses
	if len(senses) > maxCardSenses {
		senses = senses[:maxCardSenses]
	}
	if len(senses) == 1 {
		lines = append(lines, gloss(senses[0]))
	} else {
		for i, s := range senses {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, gloss(s)))
		}
	}

	examples := b.examples
	if len(examples) > maxCardExampleLines {
		examples = examples[:maxCardExampleLines]
	}
	if len(examples) > 0 {
		lines = append(lines, "")
		for _, ex := range examples {
			lines = append(lines, FormatExample(ex.chechen, ex.russian))
		}
	}
	return strings.Join(lines, "\n")
}

// chip is the grammar note after the headword: part of speech, plural ending.
func (b *block) chip() string {
	parts := make([]string, 0, 2)
	if label, ok := posLabels[b.pos]; ok {
		parts = append(parts, label)
	}
	if b.notes != "" {
		parts = append(parts, b.notes)
	}
	return strings.Join(parts, ", ")
}

func superscript(n int) string {
	digits := []rune("⁰¹²³⁴⁵⁶⁷⁸⁹")
	if n < 0 || n > 9 {
		return ""
	}
	return string(digits[n])
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

func dedupSenses(senses []string) []string {
	out := senses[:0]
	seen := map[string]bool{}
	for _, s := range senses {
		s = strings.TrimSpace(s)
		// "рука́ (кисть)" and "рука" are one sense; the fuller wording wins.
		key := NormalizeSearch(stripParens(s))
		if s == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// splitQualifiers peels leading parentheticals: "(почерк) хатӏ" → ["почерк"], "хатӏ".
func splitQualifiers(s string) (quals []string, rest string) {
	rest = strings.TrimSpace(s)
	for strings.HasPrefix(rest, "(") {
		depth, end := 0, -1
		for i, r := range rest {
			if r == '(' {
				depth++
			} else if r == ')' {
				if depth--; depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			break
		}
		quals = append(quals, strings.TrimSpace(rest[1:end]))
		rest = strings.TrimSpace(rest[end+1:])
	}
	if rest == "" {
		return nil, s
	}
	return quals, rest
}

func stripParens(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// dedupExamples drops repeats and rows that only restate the headword: the
// localization glossary stores «Куьг» → «Рука», which illustrates nothing.
func dedupExamples(examples []example, head string) []example {
	out := examples[:0]
	seen := map[string]bool{}
	for _, ex := range examples {
		key := NormalizeSearch(ex.chechen)
		if ex.chechen == "" || ex.russian == "" || seen[key] || key == head {
			continue
		}
		seen[key] = true
		out = append(out, ex)
	}
	return out
}

func dedupStrings(items []string) []string {
	out := items[:0]
	seen := map[string]bool{}
	for _, s := range items {
		key := NormalizeSearch(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

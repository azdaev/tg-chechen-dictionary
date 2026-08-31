package tools

import (
	"chetoru/internal/models"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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

// Rendered is one lookup, parsed once. Every question the handler asks of a
// set of pairs is answered here, because parsing them is the expensive part:
// the chat used to run the whole article parser twice per lookup, once for the
// card and once to learn which word to fetch grammar for.
type Rendered struct {
	// Body empty means dosham matched the query somewhere but no entry actually
	// means it — neighbours alone are not an answer, and serving them as one
	// turned «лоьма» into a bare «рядом: …» that the bot counted as a hit.
	Body       string
	Neighbours []string
	// Chechen names the Chechen word the card is about: the headword when the
	// user typed Chechen, the leading gloss when they typed Russian. Grammar
	// lives only under the Chechen headword, and dosham's search is literal —
	// «карандаш» never reaches «къолам», whose Russian side the academic corpus
	// spells «каранда́ш» — so the paradigm has to be asked for by name.
	Chechen string
	// Glossed says some block actually states what the query means. A card can
	// render without it: «собаку» brings back six sentences that contain the
	// word and no entry for it, so the card is six illustrations of a word it
	// never translates. That is worth showing when it is all there is, and worth
	// stepping past when another layer can do better.
	Glossed bool
}

func Render(query string, pairs []models.TranslationPairs) Rendered {
	c := collect(query, pairs)
	if len(c.blocks) == 0 {
		return Rendered{Neighbours: c.neighbours}
	}
	return Rendered{
		Body:       c.render(),
		Neighbours: c.neighbours,
		Chechen:    c.chechenSide(),
		Glossed:    c.glossed(),
	}
}

func (c collected) glossed() bool {
	for _, b := range c.blocks {
		if len(b.senses) > 0 {
			return true
		}
	}
	return false
}

func (c collected) chechenSide() string {
	b := c.blocks[0]
	if b.cheHead {
		return firstVariant(b.head)
	}
	if len(b.senses) > 0 {
		return firstVariant(b.senses[0])
	}
	return ""
}

// FormatCard renders one lookup as a single card, neighbours included.
func FormatCard(query string, pairs []models.TranslationPairs) string {
	r := Render(query, pairs)
	if r.Body == "" {
		return ""
	}
	if line := FormatNeighbours(r.Neighbours); line != "" {
		return r.Body + "\n\n" + line
	}
	return r.Body
}

// firstVariant takes one spelling out of a gloss: "лом, ваба (орудие)" → "лом".
func firstVariant(s string) string {
	s = stripParens(s)
	if i := strings.IndexAny(s, ",;"); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// foldPhrase is FoldSearch for whole phrases: the punctuation an entry carries
// and a query never does is dropped. dosham holds «Муха ду гӀуллакхаш? — Как
// дела?», and comparing that question mark against a query without one left
// «как дела» with an empty card and «нет перевода» under it.
func foldPhrase(s string) string {
	return trimPunct(FoldSearch(s))
}

func trimPunct(s string) string {
	return strings.Trim(s, " .,;:!?…\"'«»()")
}

// FormatNeighbours renders the "рядом" line: words the dictionary holds that
// merely start with the query.
func FormatNeighbours(neighbours []string) string {
	if len(neighbours) == 0 {
		return ""
	}
	if len(neighbours) > maxNeighbours {
		neighbours = neighbours[:maxNeighbours]
	}
	names := make([]string, len(neighbours))
	for i, n := range neighbours {
		names[i] = headCase(n)
	}
	return "<i>рядом:</i> " + strings.Join(names, ", ")
}

// HeadCase lowercases a headword's first letter. The Russian–Chechen articles
// store theirs capitalized and the other three corpora do not, so one lookup
// answered «Карандаш» and the next «телефон». Exported for the inline picker,
// which lists headwords straight from the data and had the same mix.
func HeadCase(s string) string { return headCase(s) }

func headCase(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// grammarNoteRe matches what a grammar note looks like: a Russian abbreviation
// and the ending it introduces, «мн. -аш».
var grammarNoteRe = regexp.MustCompile(`^(мн|ед|род|дат|вин|твор|предл|собир|уменьш)\.`)

// isGrammarNote reports whether a pair's note belongs in the header chip.
//
// The field carries two different things and only one of them is grammar. The
// compact corpus writes «мн. -аш»; the encyclopedic one writes a definition in
// Chechen — «Сагаран вока» for «бӏаьрг», «4 хара йолуш ду» for «дог» — and the
// chip printed those as if they were labels, so «дог» came out as a noun that
// «has four holes». Sampled over 25 words: every rate-16 note was «мн. -…» and
// every rate-100 one was prose.
func isGrammarNote(note string) bool {
	return grammarNoteRe.MatchString(strings.TrimSpace(strings.ToLower(note)))
}

// takeNote records the grammar the chip shows. The note describes dosham's own
// headword, which is always the Chechen side, so it belongs to the Chechen word
// this block leads with and to no other. Under a Russian headword the senses are
// different Chechen words: «дом» is glossed «цӏа» by the articles and «хӀусам»
// by the compact corpus, and only the second carries «мн. -аш». Taken from
// whichever pair happened to have one, the card announced «дом · рус. → чеч.,
// сущ., мн. -аш» directly above the line «1. цӏа» — a plural belonging to a word
// two lines further down. Under a Chechen headword every pair is the same word
// by construction, so any of their notes fits.
func (b *block) takeNote(note string, senses []string) {
	if b.notes != "" || !isGrammarNote(note) || len(senses) == 0 {
		return
	}
	if b.cheHead || len(b.senses) == 0 ||
		FoldSearch(firstVariant(b.senses[0])) == FoldSearch(firstVariant(senses[0])) {
		b.notes = note
	}
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

// collect turns ranked pairs into the blocks a card is made of: classify each
// pair, then file it.
func collect(query string, pairs []models.TranslationPairs) collected {
	q := newLookup(query)
	key := q.key
	var c collected
	blocks := map[string]*block{}

	// EntryIndex 0 means "no homonym number recorded", not "homonym zero", so
	// it folds into 1; otherwise one word becomes two cards.
	blockFor := func(p models.TranslationPairs, head string, cheHead bool) *block {
		idx := max(p.EntryIndex, 1)
		// Direction is part of the identity: «лом» is a Russian crowbar and a
		// Chechen lion, and one key for both put «лев» in the list of Chechen
		// translations of «лом».
		// Punctuation is not identity either: dosham holds «Ӏуьйре дика хуьлда!»
		// twice, once glossed «доброе утро» and once «доброе утро!», and keeping
		// the mark in the key printed the same greeting as two cards.
		// Nor is a qualifier: «Касатка — ж (ласточка) чӏегӏардиг» hands the card
		// the head «(ласточка) чӏегӏардиг», which the renderer already splits
		// apart for display. Keyed whole, it made a second card for a word the
		// user had just read — the same headword twice, once in brackets.
		_, name := splitQualifiers(head)
		k := fmt.Sprintf("%s\x00%d\x00%t", NormalizeSearch(trimPunct(name)), idx, cheHead)
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
		return b
	}

	for _, p := range pairs {
		switch pl := q.classify(p); pl.role {
		case roleEntry:
			b := blockFor(p, pl.head, pl.cheHead)
			b.takeNote(p.Notes, pl.senses)
			b.senses = append(b.senses, pl.senses...)
			b.examples = append(b.examples, pl.examples...)

		// Filed under an entry the user did not ask for. It gets a block of its
		// own here and loses it below, where every senseless block's examples
		// are handed to the entries they illustrate.
		case roleExample:
			b := blockFor(p, "", false)
			b.examples = append(b.examples, pl.examples...)

		case roleNeighbour:
			c.neighbours = append(c.neighbours, pl.head)
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
		b.senses = pointersLast(dedupSenses(b.senses))
		kept = append(kept, b)
	}
	kept = mergeSpellings(kept)

	// A phrase the dictionary only ever shows inside somebody else's entry is
	// still the answer. «спокойной ночи» is glossed «буьйса декъала хуьлда!»
	// twice — inside the article «Ночь» and inside «Пожелать» — and nowhere as
	// an entry of its own, so every example was orphaned and the bot answered
	// «нет перевода» while holding the translation.
	if len(kept) == 0 && len(orphaned) > 0 {
		kept = append(kept, &block{
			head:    strings.TrimSpace(Clean(query)),
			cheHead: !containsWord(orphaned[0].russian, key),
			index:   1,
		})
	}
	assignExamples(kept, orphaned)
	for _, b := range kept {
		b.examples = dedupExamples(b.examples, FoldSearch(b.head))
	}
	c.blocks = kept
	c.neighbours = dedupStrings(c.neighbours)
	return c
}

// assignExamples hands each example dosham filed under another entry to the
// block it actually illustrates. Homonyms share a headword, so the Chechen side
// cannot tell them apart — the Russian side can: «цӀа духадуста — переме́рить
// ко́мнату» belongs to the noun, «цӀа кхиа — успе́ть домо́й» to the adverb. They
// all went to whichever block sorted first, which filed «переме́рить ко́мнату»
// under «домо́й» and taught the wrong word.
func assignExamples(blocks []*block, examples []example) {
	if len(blocks) == 0 {
		return
	}
	for _, ex := range examples {
		best, score := blocks[0], 0
		if len(blocks) > 1 {
			words := foldedWords(ex.russian)
			for _, b := range blocks {
				if s := senseOverlap(b, words); s > score {
					best, score = b, s
				}
			}
		}
		best.examples = append(best.examples, ex)
	}
}

// senseOverlap counts how many of a block's gloss words the example repeats.
// Russian inflects, so a shared five-letter prefix counts too: the sense is
// «ко́мната» and the example says «ко́мнату». Five, because three would let the
// «дом» inside «домо́й» claim an example about houses.
//
// ponytail: prefix, not a stemmer — «и́мя» and «и́мени» share only two letters
// and go unmatched, so that example falls back to the first block. A real
// stemmer if this misses often enough to notice.
func senseOverlap(b *block, exampleWords []string) int {
	const inflectedPrefix = 5
	n := 0
	for _, s := range b.senses {
		for _, sw := range foldedWords(s) {
			for _, ew := range exampleWords {
				if sw == ew || sharedPrefix(sw, ew) >= inflectedPrefix {
					n++
				}
			}
		}
	}
	return n
}

// foldedWords lists the words worth comparing. Prepositions are dropped: «в»
// stands in both «домо́й, в свой дом» and «насори́ть в ко́мнате», and matching on
// it gave the adverb an example about rooms.
func foldedWords(s string) []string {
	const shortestMeaningful = 3
	var out []string
	for _, w := range strings.FieldsFunc(FoldSearch(s), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if utf8.RuneCountInString(w) >= shortestMeaningful {
			out = append(out, w)
		}
	}
	return out
}

func sharedPrefix(a, b string) int {
	ar, br := []rune(a), []rune(b)
	n := 0
	for n < len(ar) && n < len(br) && ar[n] == br[n] {
		n++
	}
	return n
}

func (c collected) render() string {
	var out []string
	for _, b := range c.blocks {
		out = append(out, b.render())
	}
	return strings.TrimSpace(strings.Join(out, "\n\n"))
}

func (b *block) render() string {
	var lines []string

	// The headword carries qualifiers too — «Шпиц» glosses «собака» as
	// "(собака) кӏезалг" — and inside the bold they read as Chechen, which is
	// the one thing the bold is there to say.
	name, headQuals := b.display()
	head := name
	if b.cheHead {
		head = "<b>" + head + "</b>"
	}
	head += b.homonym()
	if len(headQuals) > 0 {
		head += " <i>(" + strings.Join(headQuals, ", ") + ")</i>"
	}
	if chip := b.chip(); chip != "" {
		head += " · <i>" + chip + "</i>"
	}
	lines = append(lines, head)

	// Russian qualifiers — "(почерк) хатӏ" — trail the gloss rather than sit
	// inside its bold, since bold marks Chechen and nothing else.
	senses := b.glosses()
	if len(senses) == 1 {
		lines = append(lines, senses[0])
	} else {
		for i, s := range senses {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, s))
		}
	}

	examples := b.orderedExamples()
	if len(examples) > maxCardExampleLines {
		examples = examples[:maxCardExampleLines]
	}
	if len(examples) > 0 {
		if len(senses) > 0 {
			lines = append(lines, "")
		}
		for _, ex := range examples {
			lines = append(lines, FormatExample(ex.chechen, ex.russian))
		}
	}
	return strings.Join(lines, "\n")
}

// freeUsesFirst puts the examples that use the headword as a word of its own
// ahead of the ones that only use it inside a compound. Chechen builds compounds
// freely, and «нана» came back illustrated by «Нана-пӏелг дӏабуьгу еха оьзг» —
// three lines of the muscle that moves the thumb, ahead of «Нана юй хьан? —
// мать у тебя есть?». Both are real; only one of them teaches the word that was
// looked up. Stable, so within each group the dictionary's own order stands.
//
// Only for a Chechen headword. On the Russian side the same test reads as a
// preference for the nominative — «собака вцепилась» over «собаку съел» — which
// is a judgement about case, not about compounds, and not one to make quietly.
func freeUsesFirst(examples []example, head string) []example {
	if len(examples) < 2 {
		return examples
	}
	free := make([]example, 0, len(examples))
	compound := make([]example, 0, len(examples))
	for _, ex := range examples {
		if usesWordFreely(ex.chechen, NormalizeSearch(head)) {
			free = append(free, ex)
		} else {
			compound = append(compound, ex)
		}
	}
	if len(free) == 0 || len(compound) == 0 {
		return examples
	}
	return append(free, compound...)
}

// usesWordFreely reports whether text uses key as a word of its own. A hyphen
// joins what it separates: for this test «Нана-пӏелг» is one word, and not a
// use of «нана».
func usesWordFreely(text, key string) bool {
	return containsWord(strings.ReplaceAll(text, "-", ""), key)
}

// chip is the note after the headword: reading direction first, then grammar.
// Bold alone marked the Chechen side, and nothing told the user that — «телефон»
// answered «1. телефон» and there was no way to tell which of the two was which.
func (b *block) chip() string {
	parts := make([]string, 0, 3)
	if b.cheHead {
		parts = append(parts, "чеч. → рус.")
	} else {
		parts = append(parts, "рус. → чеч.")
	}
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

// packedSenseRe spots the compact corpus's own shorthand: a plural ending
// listed after the word, and the next sense glued to its number.
var (
	packedSenseRe = regexp.MustCompile(`(?:^|,\s*)-\p{Cyrillic}|\d\p{Cyrillic}`)
	senseEndingRe = regexp.MustCompile(`^-\p{Cyrillic}+$`)
	senseNumberRe = regexp.MustCompile(`^\d+`)
)

// unpackSense splits one of those strings into the senses it holds. «салам,
// -аш, 2маршалла» is two words for «привет» and a plural ending, and the card
// offered the whole string, commas and digit and all, as Chechen to say aloud.
func unpackSense(s string) []string {
	if !packedSenseRe.MatchString(s) {
		return []string{s}
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		// An ending is morphology, not a word: «кхера, -наш» is one way to say
		// «камень». Whatever the ending was glued to goes with it — the corpus
		// wrote «кхера, -нашчхар, -аш» with no separator, and there is no
		// telling where «-наш» stops and «чхар» starts.
		if part == "" || senseEndingRe.MatchString(part) {
			continue
		}
		if part = strings.TrimSpace(senseNumberRe.ReplaceAllString(part, "")); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func dedupSenses(senses []string) []string {
	unpacked := make([]string, 0, len(senses))
	for _, s := range senses {
		unpacked = append(unpacked, unpackSense(s)...)
	}
	senses = unpacked

	out := senses[:0]
	seen := map[string]bool{}
	for _, s := range senses {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// "рука́ (кисть)" and "Рука" are one sense; the first wins. Folded, not
		// normalized: the corpora disagree about stress marks, and «телефо́н»
		// beside «телефон» was reaching the card as two senses.
		//
		// ponytail: first, not richest. Preferring the longer spelling does pick
		// up «рука́ (кисть)» over «Рука», but two senses of one article collide on
		// the same key too — «дом» then led with «цӏа (учреждение)», a qualifier
		// belonging to its second sense. Needs the source rate per sense to tell
		// those apart; not worth carrying one until more than «куьг» wants it.
		key := FoldSearch(stripParens(s))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// dropRepeats removes the qualifiers that only repeat a sense the card is about
// to print anyway. «Касатка — ж (ласточка) чӏегӏардиг» labels the headword
// «(ласточка)» on a card whose first line already reads «1. ласточка»; the
// qualifier earns its place only when it says something the senses do not.
func dropRepeats(quals, senses []string) []string {
	if len(quals) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(senses))
	for _, s := range senses {
		for _, part := range unpackSense(s) {
			seen[NormalizeSearch(trimPunct(part))] = true
		}
	}
	kept := quals[:0]
	for _, q := range quals {
		if !seen[NormalizeSearch(trimPunct(q))] {
			kept = append(kept, q)
		}
	}
	return kept
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
		key := FoldSearch(ex.chechen)
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

// mergeSpellings folds together two blocks that are one word spelled twice. The
// academic corpus writes the long vowel and the stress and the compact one does
// not, so «гаьзло — левша» and «гаьзло̃ — левша́» arrived as separate entries and
// the card printed the same word twice, one line apart — 4 of 39 sampled
// Chechen cards.
//
// The meanings have to agree as well as the letters. Folded alone, «лом — лев»
// would swallow «ло̃м», and «гонахьара — описанный» would swallow
// «го̃нахьа̃ра — периферийный»: different words that a keyboard cannot tell
// apart, which is exactly why the block key normalizes rather than folds.
func mergeSpellings(blocks []*block) []*block {
	var kept []*block
	for _, b := range blocks {
		merged := false
		for _, into := range kept {
			if !oneWordTwice(into, b) {
				continue
			}
			// pointersLast ran per block before the merge, and joining two
			// lists puts the first block's pointer ahead of the second block's
			// meanings again. Re-applied so the invariant survives the join.
			into.senses = pointersLast(dedupSenses(keepRicher(append(into.senses, b.senses...))))
			into.examples = append(into.examples, b.examples...)
			if b.rate > into.rate {
				into.head, into.rate = b.head, b.rate
			}
			if into.notes == "" {
				into.notes = b.notes
			}
			if into.pos == 0 {
				into.pos = b.pos
			}
			merged = true
			break
		}
		if !merged {
			kept = append(kept, b)
		}
	}
	return kept
}

func oneWordTwice(a, b *block) bool {
	if a.index != b.index || a.cheHead != b.cheHead || len(a.senses) == 0 || len(b.senses) == 0 {
		return false
	}
	_, an := splitQualifiers(a.head)
	_, bn := splitQualifiers(b.head)
	return FoldSearch(trimPunct(an)) == FoldSearch(trimPunct(bn)) &&
		FoldSearch(firstVariant(a.senses[0])) == FoldSearch(firstVariant(b.senses[0]))
}

// keepRicher drops a sense another one already contains. The two spellings of a
// word rarely carry identical glosses — «силу — дубитель» merges with «силу̃ —
// дуби́тель, заква́ска (кожи)» — and listing both numbers the same meaning twice.
// Only inside a merge: elsewhere two senses sharing a first word are two
// senses, and «дом» would lose «цӏа (учреждение)» to «цӏа».
func keepRicher(senses []string) []string {
	out := senses[:0]
	for _, s := range senses {
		covered := false
		for i, kept := range out {
			if FoldSearch(firstVariant(s)) != FoldSearch(firstVariant(kept)) {
				continue
			}
			covered = true
			if len(s) > len(kept) {
				out[i] = s
			}
			break
		}
		if !covered {
			out = append(out, s)
		}
	}
	return out
}

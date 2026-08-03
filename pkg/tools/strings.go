package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	tagRe  = regexp.MustCompile(`<[^>]*>`)
	boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// grammarRe strips leading single-letter markers, possibly chained: gender
	// ("м цӏа"), and the palochka numbering homonyms ("ӏ ж балда" in «Губа»).
	// endingsRe strips adjective ending lists ("-ая, -ое къорден") — the comma
	// plus dash continuation keeps real words safe.
	grammarRe = regexp.MustCompile(`^([а-яёӏ]\s+)+`)
	endingsRe = regexp.MustCompile(`^-?[а-яё]{1,3}(,\s*-[а-яё]{1,3})+\s*`)
	// verbLabelRe strips the grammar labels heading a gloss ("сов., кому 1) …",
	// "несов. дала", "тк. мн. собир. …") — metadata, not translation, and
	// otherwise the first one becomes the card's header. Applied in a loop, so
	// a chain of them peels off one at a time.
	verbLabelRe = regexp.MustCompile(`^((не)?сов\.|однокр\.|многокр\.|перех\.|неперех\.|безл\.|нескл\.|т\.к\.|тк\.|мн\.|ед\.|собир\.|кратк\. ф\.|в знач\. сказ\.|кому-чему|кого-что|о ком|о чём|кому|чему|кого|кем|чем|ком|что)([,;]?\s+)`)
	// Sense markers come as "1)" but also as "ӏ. " (palochka standing in for
	// the digit) and "2. " in live dosham glosses.
	meaningRe = regexp.MustCompile(`(\d+\)|(?:^|\s)[ӏ\d]\.\s)`)
	tildeRe   = regexp.MustCompile(`~([а-яё]+)`)
)

func Clean(text string) string {
	// Most dictionary strings carry no markup at all; skip the regex for them.
	if !strings.ContainsAny(text, "<\n") {
		return text
	}
	// tagRe already consumed "<>" and "<br />" by the time the old explicit
	// replacements for them ran, so they never fired.
	output := tagRe.ReplaceAllString(text, "")
	return strings.ReplaceAll(output, "\n", " ")
}

// StripTags removes HTML tags but keeps the line structure. It backs the
// plain-text resend after Telegram rejects our markup, where Clean would be
// wrong: Clean also folds newlines into spaces, which is right for a one-line
// gloss and would turn a rejected card into a paragraph.
func StripTags(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	return tagRe.ReplaceAllString(text, "")
}

func NormalizeSearch(text string) string {
	clean := Clean(text)
	clean = strings.TrimSpace(clean)
	clean = strings.ToLower(clean)
	clean = strings.ReplaceAll(clean, "ё", "е")
	return foldPalochka(clean)
}

// foldPalochka replaces the digit-1 and Latin i/l stand-ins for the Chechen
// palochka with the real letter ("г1ала" → "гӏала") so all spellings share
// one cache key and match locally stored words. Only characters with a
// Cyrillic neighbor fold, which keeps Latin words and numbers intact.
func foldPalochka(s string) string {
	if !strings.ContainsAny(s, "1il") {
		return s
	}
	runes := []rune(s)
	isCyr := func(i int) bool {
		if i < 0 || i >= len(runes) {
			return false
		}
		r := runes[i]
		return r >= 'а' && r <= 'я' || r == 'ё' || r == 'ӏ'
	}
	for i, r := range runes {
		if (r == '1' || r == 'i' || r == 'l') && (isCyr(i-1) || isCyr(i+1)) {
			runes[i] = 'ӏ'
		}
	}
	return string(runes)
}

// maxYoVariants caps how many respellings YoVariants generates, since each
// one costs an API retry.
const maxYoVariants = 4

// YoVariants returns candidate respellings of a Russian query for a ё/е retry.
// Russians routinely type е for ё and the dictionary search does not fold the
// two. A Russian word carries at most one ё, so an all-е query yields one
// variant per е position ("береза" → "бёреза", "берёза"), and a query with ё
// yields the single all-е spelling.
func YoVariants(text string) []string {
	if strings.ContainsAny(text, "ёЁ") {
		return []string{strings.NewReplacer("ё", "е", "Ё", "Е").Replace(text)}
	}

	runes := []rune(text)
	var variants []string
	for i, r := range runes {
		var yo rune
		switch r {
		case 'е':
			yo = 'ё'
		case 'Е':
			yo = 'Ё'
		default:
			continue
		}
		v := make([]rune, len(runes))
		copy(v, runes)
		v[i] = yo
		variants = append(variants, string(v))
		if len(variants) == maxYoVariants {
			break
		}
	}
	return variants
}

func EscapeUnclosedTags(text string) string {
	// No angle brackets means no tags to balance.
	if !strings.ContainsAny(text, "<>") {
		return text
	}
	// A stray bracket ("цӏа < дитт") survives tag matching but breaks
	// Telegram's HTML parser, which rejects the whole message.
	if strings.Count(text, "<") != strings.Count(text, ">") {
		text = Clean(text)
		text = strings.ReplaceAll(text, "<", "")
		return strings.ReplaceAll(text, ">", "")
	}
	matches := tagRe.FindAllString(text, -1)
	count := 0
	for _, match := range matches {
		if strings.HasPrefix(match, "</") {
			count--
		} else {
			count++
		}
	}
	if count != 0 {
		return Clean(text) //TODO: optimize - one function for check. if true clean
	}
	return text
}

// exampleLine renders a usage example with the studied language first. Every
// card that shows an example goes through here or FormatExample, so the
// translation card and the grammar card can no longer disagree on which
// language leads.
func exampleLine(chechen, russian string) string {
	return chechen + " → " + russian
}

// FormatExample renders a usage example for a Telegram card. Italic is the
// card's mark for an example and means nothing else.
func FormatExample(chechen, russian string) string {
	return "<i>" + exampleLine(chechen, russian) + "</i>"
}

// FormatTranslationLite formats a dictionary entry into a lightweight, consistent style.
// originalWord is used for tilde replacement (~ое -> чёрное).
//
// boldGloss says the Chechen side is the gloss, not the headword — Russian
// entries carry their Chechen translation in the senses. Bold always marks the
// studied language, so it moves with it; with no language known the headword
// takes it, matching /random and /wotd.
func FormatTranslationLite(text string, originalWord string, boldGloss bool) string {
	if text == "" {
		return ""
	}

	word := strings.TrimSpace(originalWord)

	// Strip the bolded headword; we render it ourselves from originalWord.
	text = boldRe.ReplaceAllString(text, "")

	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)

	text = endingsRe.ReplaceAllString(text, "")
	// Gender marker first ("с нескл. амплуа"), then the label loop.
	text = grammarRe.ReplaceAllString(text, "")
	for {
		stripped := verbLabelRe.ReplaceAllString(text, "")
		if stripped == text {
			break
		}
		text = stripped
	}

	type sense struct {
		main     string
		examples []string
	}
	var senses []sense

	for _, part := range meaningRe.Split(text, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		semicolonIndex := findMainSemicolon(part)
		main := part
		var examples []string

		if semicolonIndex != -1 {
			main = part[:semicolonIndex]
			examples = parseExamples(part[semicolonIndex+1:], !boldGloss)
		}

		main = expandAbbreviations(cleanTranslation(main))
		if word != "" {
			main, _ = replaceTildeWithWord(main, word)
			// An example whose tilde needs a stem we cannot derive is dropped
			// rather than shown with a guess: it is an illustration, and no
			// illustration beats one built on a word that does not exist.
			kept := examples[:0]
			for _, example := range examples {
				if expanded, ok := replaceTildeWithWord(example, word); ok {
					kept = append(kept, expanded)
				}
			}
			examples = kept
		}
		for i, example := range examples {
			examples[i] = expandAbbreviations(example)
		}

		// A marker with no translation of its own ("2. ; пример - масал") is a
		// carrier for its examples, not a sense the reader should count.
		if main == "" && len(senses) > 0 {
			last := &senses[len(senses)-1]
			last.examples = append(last.examples, examples...)
			continue
		}
		senses = append(senses, sense{main: main, examples: examples})
	}

	bold := func(s string) string {
		if s == "" {
			return s
		}
		return "<b>" + s + "</b>"
	}
	header := word
	if !boldGloss {
		header = bold(header)
	}
	gloss := func(s string) string {
		if boldGloss {
			return bold(s)
		}
		return s
	}

	// parseExamples caps each sense at five, which is fine until a word has
	// twenty-six of them: «Идти» ran to fifty-eight lines, well past what anyone
	// scrolls. The budget is spent in sense order, so the examples that survive
	// are the ones under the senses the reader meets first.
	budget := maxCardExamples
	omitted := 0
	take := func(examples []string) []string {
		// Per sense as well as per card: spending it greedily let «Идти» give all
		// six to sense one — which spends them on parenthetical fragments — and
		// nothing to the twenty-one senses under it.
		limit := budget
		if len(senses) > 1 && limit > maxExamplesPerSense {
			limit = maxExamplesPerSense
		}
		if len(examples) > limit {
			omitted += len(examples) - limit
			examples = examples[:limit]
		}
		budget -= len(examples)
		return examples
	}
	withTail := func(lines []string) string {
		if omitted > 0 {
			lines = append(lines, fmt.Sprintf("<i>…ещё %d прим.</i>", omitted))
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var lines []string
	// One sense stays a one-liner — the common case, and a numbered list of one
	// is noise. Two or more put the headword on its own line so the numbers
	// start at 1 under it.
	if len(senses) == 1 && senses[0].main != "" {
		switch {
		case word != "":
			lines = append(lines, header+" — "+gloss(senses[0].main))
		default:
			lines = append(lines, gloss(senses[0].main))
		}
		return withTail(append(lines, renderExamples(take(senses[0].examples))...))
	}

	if word != "" {
		lines = append(lines, header)
	}
	number := 0
	for _, s := range senses {
		if s.main != "" {
			number++
			lines = append(lines, fmt.Sprintf("%d. %s", number, gloss(s.main)))
		}
		lines = append(lines, renderExamples(take(s.examples))...)
	}

	return withTail(lines)
}

const (
	// maxCardExamples caps how many usage examples one card may carry, across
	// all its senses; maxExamplesPerSense keeps one sense from taking them all.
	maxCardExamples     = 6
	maxExamplesPerSense = 2
)

// renderExamples prefixes a sense's examples. A lone example sits flush under
// its sense; three or more get an indent (Telegram keeps leading spaces) so a
// long list reads as belonging to the sense above it rather than to the card.
func renderExamples(examples []string) []string {
	prefix := "• "
	if len(examples) >= 3 {
		prefix = "   • "
	}
	out := make([]string, 0, len(examples))
	for _, example := range examples {
		out = append(out, prefix+example)
	}
	return out
}

// FirstExample mines a raw dictionary gloss for its first usage example and
// renders it as "chechen → russian". chechenLeads says word is the Chechen
// headword, so the source order already holds; it is false for a Russian entry
// whose gloss is Chechen. ok is false when no sense carries a splittable
// example. The result is plain text — callers escape it before wrapping.
func FirstExample(gloss, word string, chechenLeads bool) (string, bool) {
	gloss = boldRe.ReplaceAllString(gloss, "")
	for _, sense := range meaningRe.Split(gloss, -1) {
		idx := findMainSemicolon(sense)
		if idx == -1 {
			continue
		}
		for part := range strings.SplitSeq(sense[idx+1:], ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			left, right, ok := splitExample(part)
			if !ok || left == "" || right == "" {
				continue
			}
			if !chechenLeads {
				left, right = right, left
			}
			ex := exampleLine(left, right)
			if word != "" {
				expanded, exact := replaceTildeWithWord(ex, word)
				if !exact {
					continue // keep looking; a later example may not need a stem
				}
				ex = expanded
			}
			return expandAbbreviations(ex), true
		}
	}
	return "", false
}

// findMainSemicolon returns the index of the semicolon that separates the main
// translation from its examples: the first one followed by a dash (the example
// marker), falling back to the first semicolon, or -1 if there is none.
func findMainSemicolon(text string) int {
	semicolons := []int{}
	for i, r := range text {
		if r == ';' {
			semicolons = append(semicolons, i)
		}
	}

	for _, pos := range semicolons {
		if strings.ContainsAny(text[pos+1:], "-–—") {
			return pos
		}
	}

	if len(semicolons) > 0 {
		return semicolons[0]
	}

	return -1
}

func cleanTranslation(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)
	return text
}

// parseExamples splits a semicolon-separated example list into rendered lines,
// capped at 5. Each example reads "headword phrase - gloss translation", so
// which side is Chechen follows from the entry, not from the text: chechenLeads
// says the headword side is the Chechen one and the source order already holds.
func parseExamples(text string, chechenLeads bool) []string {
	var examples []string

	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if left, right, ok := splitExample(part); ok {
			if left != "" && right != "" {
				if chechenLeads {
					examples = append(examples, FormatExample(left, right))
				} else {
					examples = append(examples, FormatExample(right, left))
				}
			}
		} else {
			// Not a two-sided example — a whole-sentence illustration. Still an
			// example, so it still gets the card's italic.
			examples = append(examples, "<i>"+part+"</i>")
		}
	}

	if len(examples) > 5 {
		examples = examples[:5]
	}

	return examples
}

// splitExample splits one example at the dash separating its two sides. The
// source data mixes hyphens with en/em dashes, so all three count, and the
// separator needs a space on at least one side: live glosses glue it to the
// preceding tilde ("деревянный ~- дечиган цӏа") but never write the
// case-government shorthand ("кого-л.", "что-л.") with any space at all, and
// splitting on that one turns «проводить кого-л. до дома» into «л. до дома →
// проводить кого». ok is false when no dash qualifies.
func splitExample(part string) (left, right string, ok bool) {
	for i, r := range part {
		if r != '-' && r != '–' && r != '—' {
			continue
		}
		width := utf8.RuneLen(r)
		if i == 0 || i+width >= len(part) {
			continue // nothing to put on one of the sides
		}
		if part[i-1] != ' ' && part[i+width] != ' ' {
			continue // inside a word
		}
		left = strings.Trim(strings.TrimSpace(part[:i]), `"«»""`)
		right = strings.Trim(strings.TrimSpace(part[i+width:]), `"«»""`)
		return left, right, true
	}
	return "", "", false
}

// replaceTildeWithWord expands the dictionary's tilde shorthand for word. exact
// is false when some tilde needed a stem wordStem refuses to guess; callers that
// can drop the line should, and the rest get the headword uninflected — wrong
// case, but a word of the language rather than an invented one.
func replaceTildeWithWord(text, word string) (string, bool) {
	if word == "" {
		return text, true
	}

	lowerWord := strings.ToLower(word)
	stem, regular := wordStem(lowerWord)
	exact := true
	result := tildeRe.ReplaceAllStringFunc(text, func(match string) string {
		ending := match[1:]
		// Русские грамматические окончания не длиннее 3 букв. Более длинный
		// «хвост» — это отдельное слово, склеенное в источнике с заглавным
		// (например «~культуры» для слова «дом» значит «дом культуры»).
		// Подставляем слово с пробелом, а не лепим несуществующее «домкультуры».
		if len([]rune(ending)) >= 4 {
			return lowerWord + " " + ending
		}
		if !regular {
			exact = false
			return lowerWord
		}
		return stem + ending
	})

	// Замена одиночной тильды (~) на само слово в именительном падеже.
	// Одиночная тильда обозначает заглавное слово без изменений, поэтому
	// подставляем полную форму, а не основу.
	return strings.ReplaceAll(result, "~", lowerWord), exact
}

// wordStem cuts a headword back to the part a short grammatical ending attaches
// to, and reports whether the cut is one Russian spelling actually determines.
//
// Two are. An adjective in -ый/-ий/-ой drops those two letters for every form
// it has. A word ending in a vowel drops it — «слеза» + «~ами» is «слезами»,
// «домашний» + «~ие» is «домашние».
//
// The rest are morphology the spelling does not carry, and the old heuristic
// invented words there: a consonant-final headword may hide a fleeting vowel
// («силок» + «~ки» is «силки», not «силокки») or a suffix the ending replaces
// («разбойник» + «~ца» is «разбойница»), and a verb's infinitive is not its
// present stem («визжать» + «~ит» is «визжит», not «визжаит»). Measured on
// 1800 live entries, refusing these two classes drops every wrong expansion in
// the sample and about a dozen right ones.
func wordStem(word string) (string, bool) {
	runes := []rune(word)
	if len(runes) < 3 {
		return word, false
	}
	switch string(runes[len(runes)-2:]) {
	case "ый", "ий", "ой":
		return string(runes[:len(runes)-2]), true
	case "ся":
		// A reflexive verb ends in a vowel but is not a vowel stem: «застояться»
		// + «~лся» is «застоялся», and cutting the last letter writes
		// «застоятьслся».
		return word, false
	}
	if strings.ContainsRune("аеёиоуыэюя", runes[len(runes)-1]) {
		return string(runes[:len(runes)-1]), true
	}
	return word, false
}

// expandAbbreviations заменяет словарные сокращения на полные формы
func expandAbbreviations(text string) string {
	return abbreviationReplacer.Replace(text)
}

// abbreviationReplacer expands dictionary abbreviations in one pass. Pairs are
// ordered longest-first (ties alphabetical, so the order is deterministic)
// because short abbreviations can be substrings of longer ones ("им." inside
// "хим."). Expansions contain no periods while every abbreviation ends with
// one, so a single pass cannot create new matches.
var abbreviationReplacer = newAbbreviationReplacer()

func newAbbreviationReplacer() *strings.Replacer {
	abbreviations := map[string]string{
		"тж.": "также",
		// Both spellings occur live: "тк. мн." in «Нечистоты», "т.к. кратк. ф."
		// in «Длинный». In a dictionary gloss it is "только", not "так как".
		"тк.":  "только",
		"т.к.": "только",
		// "кратк." earns an entry even though the card rarely shows it: without
		// one the replacer finds "тк." inside it and writes "кратолько".
		// Longest-first ordering then lets the whole phrase win.
		"кратк. ф.":  "(краткая форма)",
		"кратк.":     "(краткая форма)",
		"вводн. сл.": "(вводное слово)",
		"разг.":      "(разговорное)",
		"прост.":     "(просторечие)",
		"перен.":     "(переносное)",
		"устар.":     "(устаревшее)",
		"книжн.":     "(книжное)",
		"офиц.":      "(официальное)",
		"спец.":      "(специальное)",
		"мед.":       "(медицинское)",
		"воен.":      "(военное)",
		"юр.":        "(юридическое)",
		"тех.":       "(техническое)",
		"муз.":       "(музыкальное)",
		"мат.":       "(математическое)",
		"физ.":       "(физическое)",
		"хим.":       "(химическое)",
		"биол.":      "(биологическое)",
		"геол.":      "(геологическое)",
		"бот.":       "(ботаническое)",
		"зоол.":      "(зоологическое)",
		"геогр.":     "(географическое)",
		"ист.":       "(историческое)",
		"эк.":        "(экономическое)",
		"полит.":     "(политическое)",
		"рел.":       "(религиозное)",
		"филос.":     "(философское)",
		"лит.":       "(литературное)",
		"поэт.":      "(поэтическое)",
		"ирон.":      "(ироничное)",
		"шутл.":      "(шутливое)",
		"пренебр.":   "(пренебрежительное)",
		"ласк.":      "(ласкательное)",
		"уменьш.":    "уменьшительное",
		"увелич.":    "(увеличительное)",
		"собир.":     "(собирательное)",
		"множ.":      "(множественное)",
		"ед.":        "(единственное)",
		"мн.":        "(множественное)",
		"им.":        "(именительный)",
		"род.":       "(родительный)",
		"дат.":       "(дательный)",
		"вин.":       "(винительный)",
		"тв.":        "(творительный)",
		"пр.":        "(предложный)",
	}

	keys := make([]string, 0, len(abbreviations))
	for abbrev := range abbreviations {
		keys = append(keys, abbrev)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})

	pairs := make([]string, 0, len(keys)*2)
	for _, abbrev := range keys {
		pairs = append(pairs, abbrev, abbreviations[abbrev])
	}
	return strings.NewReplacer(pairs...)
}

package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	// verbLabelRe strips the aspect/government labels heading verb glosses
	// ("сов., кому 1) …", "несов. дала") — grammar metadata, not translation,
	// and otherwise it becomes the card's header.
	verbLabelRe = regexp.MustCompile(`^((не)?сов\.|однокр\.|многокр\.|перех\.|неперех\.|безл\.|нескл\.|кому-чему|кого-что|о ком|о чём|кому|чему|кого|кем|чем|ком|что)([,;]?\s+)`)
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
	output := tagRe.ReplaceAllString(text, "")
	output = strings.ReplaceAll(output, "<>", ";")
	output = strings.ReplaceAll(output, "<br />", " ")
	output = strings.ReplaceAll(output, "\n", " ")
	return output
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
			main = replaceTildeWithWord(main, word)
			for i, example := range examples {
				examples[i] = replaceTildeWithWord(example, word)
			}
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
		lines = append(lines, renderExamples(senses[0].examples)...)
		return strings.TrimSpace(strings.Join(lines, "\n"))
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
		lines = append(lines, renderExamples(s.examples)...)
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

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
				ex = replaceTildeWithWord(ex, word)
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

// splitExample splits one "russian - chechen" example at its first dash. The
// source data mixes hyphens with en/em dashes, so all three count. ok is false
// when there is no inner dash to split on.
func splitExample(part string) (russian, chechen string, ok bool) {
	idx, width := -1, 0
	for _, d := range []string{"-", "–", "—"} {
		if i := strings.Index(part, d); i != -1 && (idx == -1 || i < idx) {
			idx, width = i, len(d)
		}
	}
	if idx <= 0 || idx+width >= len(part) {
		return "", "", false
	}
	russian = strings.Trim(strings.TrimSpace(part[:idx]), `"«»""`)
	chechen = strings.Trim(strings.TrimSpace(part[idx+width:]), `"«»""`)
	return russian, chechen, true
}

// replaceTildeWithWord заменяет тильду (~) в тексте на основное слово
func replaceTildeWithWord(text, word string) string {
	if word == "" {
		return text
	}

	wordBase := getWordBase(word)
	lowerWord := strings.ToLower(word)

	result := tildeRe.ReplaceAllStringFunc(text, func(match string) string {
		ending := match[1:]
		// Русские грамматические окончания не длиннее 3 букв. Более длинный
		// «хвост» — это отдельное слово, склеенное в источнике с заглавным
		// (например «~культуры» для слова «дом» значит «дом культуры»).
		// Подставляем слово с пробелом, а не лепим несуществующее «домкультуры».
		if len([]rune(ending)) >= 4 {
			return lowerWord + " " + ending
		}
		// Грамматическое окончание, склеиваемое с основой («~а» для «дом» → «дома»).
		return wordBase + ending
	})

	// Замена одиночной тильды (~) на само слово в именительном падеже.
	// Одиночная тильда обозначает заглавное слово без изменений, поэтому
	// подставляем полную форму, а не основу.
	result = strings.ReplaceAll(result, "~", lowerWord)

	return result
}

// getWordBase получает основу слова для склонения
func getWordBase(word string) string {
	word = strings.ToLower(word)

	runes := []rune(word)
	if len(runes) < 2 {
		return word
	}

	// Прилагательные на -ый, -ий, -ой → убираем 2 символа
	last2 := string(runes[len(runes)-2:])
	if last2 == "ый" || last2 == "ий" || last2 == "ой" {
		return string(runes[:len(runes)-2])
	}

	// Существительные/глаголы на гласную → убираем 1 символ
	lastRune := string(runes[len(runes)-1])
	if strings.Contains("аеёиоуыэюя", lastRune) {
		return string(runes[:len(runes)-1])
	}

	return word
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
		"тж.":        "также",
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

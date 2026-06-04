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
	// grammarRe strips single-letter gender/number markers ("м цӏа");
	// endingsRe strips adjective ending lists ("-яя, -ее цӏера") — the comma
	// plus dash continuation keeps real words safe.
	grammarRe = regexp.MustCompile(`^[а-яё]\s+`)
	endingsRe = regexp.MustCompile(`^[а-яё]{1,3}(,\s*-[а-яё]{1,3})+\s*`)
	// verbLabelRe strips the aspect/government labels heading verb glosses
	// ("сов., кому 1) …", "несов. дала") — grammar metadata, not translation,
	// and otherwise it becomes the card's header.
	verbLabelRe = regexp.MustCompile(`^((не)?сов\.|однокр\.|многокр\.|перех\.|неперех\.|безл\.|кому-чему|кого-что|о ком|о чём|кому|чему|кого|кем|чем|ком|что)([,;]?\s+)`)
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

// FormatTranslationLite formats a dictionary entry into a lightweight, consistent style.
// originalWord is used for tilde replacement (~ое -> чёрное)
func FormatTranslationLite(text string, originalWord string) string {
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
	for {
		stripped := verbLabelRe.ReplaceAllString(text, "")
		if stripped == text {
			break
		}
		text = stripped
	}
	text = grammarRe.ReplaceAllString(text, "")

	parts := meaningRe.Split(text, -1)

	var lines []string
	headerWritten := false

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		semicolonIndex := findMainSemicolon(part)
		main := part
		examples := []string{}

		if semicolonIndex != -1 {
			main = part[:semicolonIndex]
			examples = parseExamples(part[semicolonIndex+1:])
		}

		main = expandAbbreviations(cleanTranslation(main))
		if word != "" {
			main = replaceTildeWithWord(main, word)
		}

		if !headerWritten {
			if word != "" && main != "" {
				lines = append(lines, fmt.Sprintf("%s — %s", word, main))
			} else if word != "" {
				lines = append(lines, word)
			} else if main != "" {
				lines = append(lines, main)
			}
			headerWritten = true
		} else if main != "" {
			lines = append(lines, fmt.Sprintf("• %s", main))
		}

		for _, example := range examples {
			if word != "" {
				example = replaceTildeWithWord(example, word)
			}
			example = expandAbbreviations(example)
			lines = append(lines, fmt.Sprintf("• %s", example))
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
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

// parseExamples splits a "russian phrase - chechen translation" list (separated
// by semicolons) into rendered "russian → chechen" lines, capped at 5.
func parseExamples(text string) []string {
	var examples []string

	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if russian, chechen, ok := splitExample(part); ok {
			if russian != "" && chechen != "" {
				examples = append(examples, fmt.Sprintf("%s → %s", russian, chechen))
			}
		} else {
			examples = append(examples, part)
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
		// Винительный ед.ч. для слов типа «слово»: полная форма, не основа.
		if ending == "о" {
			return lowerWord
		}
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

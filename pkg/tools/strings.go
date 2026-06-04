package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func Clean(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	output := re.ReplaceAllString(text, "")
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
	return clean
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
	re := regexp.MustCompile(`<[^>]*>`)
	matches := re.FindAllString(text, -1)
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
	wordRe := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = wordRe.ReplaceAllString(text, "")

	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)

	grammarRe := regexp.MustCompile(`^[а-яё]\s+`)
	text = grammarRe.ReplaceAllString(text, "")

	meaningRe := regexp.MustCompile(`(\d+\))`)
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
		if strings.Contains(text[pos+1:], "-") {
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

		if strings.Contains(part, "-") {
			dashIndex := strings.Index(part, "-")
			if dashIndex > 0 && dashIndex < len(part)-1 {
				russian := strings.Trim(strings.TrimSpace(part[:dashIndex]), `"«»""`)
				chechen := strings.Trim(strings.TrimSpace(part[dashIndex+1:]), `"«»""`)
				if russian != "" && chechen != "" {
					examples = append(examples, fmt.Sprintf("%s → %s", russian, chechen))
				}
			} else {
				examples = append(examples, part)
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

// replaceTildeWithWord заменяет тильду (~) в тексте на основное слово
func replaceTildeWithWord(text, word string) string {
	if word == "" {
		return text
	}

	wordBase := getWordBase(word)
	lowerWord := strings.ToLower(word)

	// Словарь популярных окончаний для правильного склонения
	commonEndings := map[string]string{
		"а":   wordBase + "а",   // родительный ед.ч.
		"у":   wordBase + "у",   // дательный ед.ч.
		"ом":  wordBase + "ом",  // творительный ед.ч.
		"е":   wordBase + "е",   // предложный ед.ч.
		"ой":  wordBase + "ой",  // творительный ед.ч. (жен.род)
		"ах":  wordBase + "ах",  // предложный мн.ч.
		"ами": wordBase + "ами", // творительный мн.ч.
		"ы":   wordBase + "ы",   // именительный мн.ч.
		"и":   wordBase + "и",   // именительный мн.ч. / родительный ед.ч. (жен.род)
		"ях":  wordBase + "ях",  // предложный мн.ч. (мягкая основа)
		"ями": wordBase + "ями", // творительный мн.ч. (мягкая основа)
		"ов":  wordBase + "ов",  // родительный мн.ч. (муж.род)
		"ев":  wordBase + "ев",  // родительный мн.ч. (мягкая основа)
		"ам":  wordBase + "ам",  // дательный мн.ч.
		"ём":  wordBase + "ём",  // творительный ед.ч. (мягкая основа)
		"о":   lowerWord,        // винительный ед.ч. (для слов типа "слово")
	}

	result := text

	tildeRe := regexp.MustCompile(`~([а-яё]+)`)
	result = tildeRe.ReplaceAllStringFunc(result, func(match string) string {
		ending := match[1:]
		if replacement, exists := commonEndings[ending]; exists {
			return replacement
		}
		// Русские грамматические окончания не длиннее 3 букв. Более длинный
		// «хвост» — это отдельное слово, склеенное в источнике с заглавным
		// (например «~культуры» для слова «дом» значит «дом культуры»).
		// Подставляем слово с пробелом, а не лепим несуществующее «домкультуры».
		if len([]rune(ending)) >= 4 {
			return lowerWord + " " + ending
		}
		// Иначе считаем это нераспознанным окончанием на основе слова.
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
	// Словарь сокращений и их расшифровок
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

	result := text

	// Заменяем сокращения, начиная с самых длинных. Порядок обхода map в Go
	// случаен, а короткие сокращения могут быть подстрокой длинных
	// (например "им." внутри "хим."), поэтому обрабатываем длинные первыми и
	// детерминированно, чтобы избежать ошибочных частичных замен.
	keys := make([]string, 0, len(abbreviations))
	for abbrev := range abbreviations {
		keys = append(keys, abbrev)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j] // total order: ties are alphabetical, never random
	})

	for _, abbrev := range keys {
		result = strings.ReplaceAll(result, abbrev, abbreviations[abbrev])
	}

	return result
}

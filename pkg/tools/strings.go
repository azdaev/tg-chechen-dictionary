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

func AlternateYo(text string) string {
	if strings.ContainsAny(text, "ёЁ") {
		return ""
	}
	if !strings.ContainsAny(text, "еЕ") {
		return ""
	}

	replacer := strings.NewReplacer("е", "ё", "Е", "Ё")
	return replacer.Replace(text)
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

// FormatTranslation форматирует словарную статью для красивого отображения в боте
func FormatTranslation(text string) string {
	if text == "" {
		return ""
	}

	// Извлекаем основное слово (в жирном шрифте или в начале)
	wordRe := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	wordMatch := wordRe.FindStringSubmatch(text)

	var word string
	if len(wordMatch) > 1 {
		word = strings.TrimSpace(wordMatch[1])
		// Удаляем основное слово из текста для дальнейшей обработки
		text = wordRe.ReplaceAllString(text, "")
	}

	// Удаляем тире после основного слова
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)

	// Удаляем грамматические пометы (одну букву в начале, например "ж", "с", "м")
	grammarRe := regexp.MustCompile(`^[а-яё]\s+`)
	text = grammarRe.ReplaceAllString(text, "")

	// Разделяем по цифрам с скобками (1), 2), 3) и т.д.)
	meaningRe := regexp.MustCompile(`(\d+\))`)
	parts := meaningRe.Split(text, -1)
	meaningNumbers := meaningRe.FindAllString(text, -1)

	var result strings.Builder

	if word != "" {
		result.WriteString(fmt.Sprintf("📝 %s\n\n", strings.ToUpper(word)))
	}

	meaningIndex := 1
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Добавляем номер значения
		if i > 0 && i-1 < len(meaningNumbers) {
			result.WriteString(fmt.Sprintf("%s ", getNumberEmoji(meaningIndex)))
			meaningIndex++
		} else if i == 0 && len(meaningNumbers) == 0 {
			// Если всего одно значение без нумерации
			result.WriteString("1️⃣ ")
		} else if i == 0 {
			continue // Пропускаем пустую часть до первой цифры
		}

		// Парсим основной перевод и примеры
		meaning := part

		// Разделяем основной перевод и примеры
		// Ищем первую точку с запятой, которая не является частью примера
		semicolonIndex := findMainSemicolon(meaning)

		if semicolonIndex == -1 {
			// Нет примеров, только основной перевод
			mainTranslation := cleanTranslation(meaning)
			mainTranslation = expandAbbreviations(mainTranslation)
			result.WriteString(mainTranslation)
		} else {
			// Есть основной перевод и примеры
			mainTranslation := cleanTranslation(meaning[:semicolonIndex])
			mainTranslation = expandAbbreviations(mainTranslation)
			examples := meaning[semicolonIndex+1:]

			result.WriteString(mainTranslation)

			// Парсим примеры
			examplesList := parseExamples(examples)
			if len(examplesList) > 0 {
				result.WriteString("\n")
				for _, example := range examplesList {
					// Заменяем тильду на основное слово в примерах
					if word != "" {
						example = replaceTildeWithWord(example, word)
					}
					// Заменяем сокращения в примерах
					example = expandAbbreviations(example)
					result.WriteString(fmt.Sprintf("   • %s\n", example))
				}
			}
		}

		if i < len(parts)-1 && strings.TrimSpace(parts[i+1]) != "" {
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String())
}

// FormatTranslationLite formats a dictionary entry into a lightweight, consistent style.
// originalWord is used for tilde replacement (~ое -> чёрное)
func FormatTranslationLite(text string, originalWord string) string {
	if text == "" {
		return ""
	}

	// Use provided original word, or extract from bolded text as fallback
	word := strings.TrimSpace(originalWord)
	
	// Remove bolded word from text if present
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

// findMainSemicolon находит первую точку с запятой, которая разделяет основной перевод и примеры
func findMainSemicolon(text string) int {
	// Ищем первую точку с запятой, после которой есть тире (признак примера)
	semicolons := []int{}
	for i, r := range text {
		if r == ';' {
			semicolons = append(semicolons, i)
		}
	}

	for _, pos := range semicolons {
		afterSemicolon := text[pos+1:]
		// Если после точки с запятой есть тире, то это начало примеров
		if strings.Contains(afterSemicolon, "-") {
			return pos
		}
	}

	// Если не нашли подходящую точку с запятой, возвращаем первую
	if len(semicolons) > 0 {
		return semicolons[0]
	}

	return -1
}

// cleanTranslation очищает основной перевод от лишних символов
func cleanTranslation(text string) string {
	// Убираем лишние пробелы и знаки препинания
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)
	return text
}

// parseExamples извлекает примеры в формате "русская фраза - чеченский перевод"
func parseExamples(text string) []string {
	var examples []string

	// Сначала разделим по точкам с запятой
	parts := strings.Split(text, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Проверяем, есть ли тире в части
		if strings.Contains(part, "-") {
			// Пытаемся разделить по тире
			dashIndex := strings.Index(part, "-")
			if dashIndex > 0 && dashIndex < len(part)-1 {
				russian := strings.TrimSpace(part[:dashIndex])
				chechen := strings.TrimSpace(part[dashIndex+1:])

				// Убираем кавычки если есть
				russian = strings.Trim(russian, `"«»""`)
				chechen = strings.Trim(chechen, `"«»""`)

				if russian != "" && chechen != "" {
					examples = append(examples, fmt.Sprintf("%s → %s", russian, chechen))
				}
			} else {
				// Если не удалось разделить по тире, добавляем как есть
				examples = append(examples, part)
			}
		} else {
			// Если нет тире, это может быть просто предложение-пример
			examples = append(examples, part)
		}
	}

	// Ограничиваем количество примеров до 5
	if len(examples) > 5 {
		examples = examples[:5]
	}

	return examples
}

// getNumberEmoji возвращает эмодзи цифры для нумерации значений
func getNumberEmoji(num int) string {
	emojis := []string{"", "1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣", "6️⃣", "7️⃣", "8️⃣", "9️⃣", "🔟"}
	if num > 0 && num < len(emojis) {
		return emojis[num]
	}
	return fmt.Sprintf("%d️⃣", num)
}

// replaceTildeWithWord заменяет тильду (~) в тексте на основное слово
func replaceTildeWithWord(text, word string) string {
	if word == "" {
		return text
	}

	// Получаем основу слова для правильного склонения
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

	// Сначала заменяем конкретные окончания из словаря
	tildeRe := regexp.MustCompile(`~([а-яё]+)`)
	result = tildeRe.ReplaceAllStringFunc(result, func(match string) string {
		ending := match[1:] // убираем тильду
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

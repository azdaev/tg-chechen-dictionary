package tools

import (
	"fmt"
	"regexp"
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
			result.WriteString(fmt.Sprintf("%s%s ", getNumberEmoji(meaningIndex), meaningNumbers[i-1]))
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
			result.WriteString(mainTranslation)
		} else {
			// Есть основной перевод и примеры
			mainTranslation := cleanTranslation(meaning[:semicolonIndex])
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

	// Заменяем различные варианты тильды
	result := text

	// Простая замена ~а, ~у, ~ой и т.д. на основу + окончание
	tildeRe := regexp.MustCompile(`~([а-яё]+)`)
	result = tildeRe.ReplaceAllStringFunc(result, func(match string) string {
		ending := match[1:] // убираем тильду
		if ending == "" {
			return strings.ToLower(word) // если нет окончания, возвращаем полное слово в нижнем регистре
		}
		return wordBase + ending
	})

	// Замена одиночной тильды на полное слово (которая не была заменена выше)
	result = strings.ReplaceAll(result, "~", strings.ToLower(word))

	return result
}

// getWordBase получает основу слова для склонения
func getWordBase(word string) string {
	word = strings.ToLower(word)

	// Работаем с рунами для правильной обработки UTF-8
	runes := []rune(word)
	if len(runes) > 1 {
		lastRune := string(runes[len(runes)-1])
		if strings.Contains("аеёиоуыэюя", lastRune) {
			return string(runes[:len(runes)-1])
		}
	}

	return word
}

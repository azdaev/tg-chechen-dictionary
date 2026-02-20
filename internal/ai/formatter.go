package ai

import (
	"context"
	"fmt"
	"strings"
)

// FormatDictionaryEntry formats a raw dictionary entry using AI.
// Returns formatted text optimized for readability in Telegram.
func (c *Client) FormatDictionaryEntry(ctx context.Context, rawEntry string) (string, error) {
	// Escape any existing quotes in the entry
	escapedEntry := strings.ReplaceAll(rawEntry, `"`, `\"`)

	prompt := `Отформатируй словарную статью чеченского словаря для отображения в Telegram-боте.

ВХОДНЫЕ ДАННЫЕ:
"` + escapedEntry + `"

ТРЕБОВАНИЯ К ФОРМАТИРОВАНИЮ:
1. **Заголовок**: слово — основной перевод (без жирного шрифта)
2. **Группировка значений**: если есть 1), 2), 3) — показать через эмодзи цифр (1️⃣ 2️⃣ 3️⃣) с пояснениями в скобках
3. **Примеры**: под каждым значением с отступом через • (bullet point)
4. **Тильда (~)**: заменить на основное слово в правильной форме
5. **Чистота**: убрать лишние пометы, скобки, сокращения если не критично
6. **Единообразие**: всегда одинаковая структура

ПРИМЕР ВХОДА:
**чёрный** - ая, -ое ӏ. в разн. знач. ӏаьржа; ~ое платье - ӏаьржа коч; ~ый хлеб - ӏаьржа бепиг; ~ая работа - ӏаьржа болх; ~ая буря - лаьттан дарц 2. в знач. сущ. ~ое с ӏаьржа бедар; ходить в ~ом - ӏаьржа духар лело 3. в знач. сущ. ~ые (множественное) (шашки и т. п.) ӏаьржанаш

ПРИМЕР ВЫХОДА:
чёрный — ӏаьржа

1️⃣ (в разных значениях)
   • чёрное платье → ӏаьржа коч
   • чёрный хлеб → ӏаьржа бепиг
   • чёрная работа → ӏаьржа болх
   • чёрная буря → лаьттан дарц

2️⃣ (существительное)
   • чёрное → ӏаьржа бедар
   • ходить в чёрном → ӏаьржа духар лело

3️⃣ (множественное число)
   • чёрные (шашки) → ӏаьржанаш

Верни ТОЛЬКО отформатированный текст без пояснений.`

	content, err := c.complete(ctx, []message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("ai format failed: %w", err)
	}

	// Clean up the response
	formatted := strings.TrimSpace(content)
	
	// Remove markdown code blocks if present
	formatted = strings.TrimPrefix(formatted, "```")
	formatted = strings.TrimSuffix(formatted, "```")
	formatted = strings.TrimSpace(formatted)

	return formatted, nil
}

// SpellCheckResult contains the structured result of a spellcheck.
type SpellCheckResult struct {
	Corrected   string // corrected text only (empty if no corrections)
	Explanation string // full response with explanations
	NoErrors    bool   // true if no errors were found
}

// SpellCheck checks and corrects Chechen text using AI.
func (c *Client) SpellCheck(ctx context.Context, text string) (*SpellCheckResult, error) {
	prompt := `Ты — корректор чеченского языка. Проверь текст на чеченском и исправь ошибки.

ТЕКСТ:
"` + strings.ReplaceAll(text, `"`, `\"`) + `"

ПРАВИЛА:
1. Исправь орфографические ошибки
2. Исправь использование специальных букв (ӏ, аь, оь, уь, юь, яь, хь, кх, гӏ, etc.)
3. Если текст на русском или другом языке — напиши "Это не чеченский текст"
4. Если ошибок нет — верни ТОЛЬКО: NO_ERRORS

ФОРМАТ ОТВЕТА (если есть исправления):
CORRECTED: <исправленный текст>
CHANGES:
• <что было> → <что стало>

Верни ТОЛЬКО ответ в указанном формате.`

	content, err := c.complete(ctx, []message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("ai spellcheck failed: %w", err)
	}

	raw := strings.TrimSpace(content)

	if strings.Contains(raw, "NO_ERRORS") {
		return &SpellCheckResult{NoErrors: true}, nil
	}

	result := &SpellCheckResult{Explanation: raw}

	// Extract corrected text
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CORRECTED:") {
			result.Corrected = strings.TrimSpace(strings.TrimPrefix(line, "CORRECTED:"))
			break
		}
	}

	return result, nil
}

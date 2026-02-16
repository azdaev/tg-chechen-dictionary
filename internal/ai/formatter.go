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
1. **Основное слово** — выделить жирным, затем главный перевод через тире
2. Если есть несколько значений (1), 2), 3)) — показать их списком через • (не больше 3 значений)
3. Примеры (фразы с переводом) — показать через • после основного значения
4. Убрать лишние грамматические пометы (ж, м, с) — оставить только если это важно для понимания
5. Заменить тильду (~) на основное слово в нужной форме
6. Максимальная читаемость — без лишних символов и скобок
7. Формат: чисто и единообразно, как в современных словарях

ПРИМЕР ВХОДА:
**чёрный** - ая, -ое ӏ. в разн. знач. ӏаьржа; чёрное платье → ӏаьржа коч; чёрный хлеб → ӏаьржа бепиг; чёрная работа → ӏаьржа болх; чёрная буря → лаьттан дарц 2. в знач. сущ. чёрное с ӏаьржа бедар; ходить в чёрном → ӏаьржа духар лело 3. в знач. сущ. чёрные (множественное) (шашки и т. п.) ӏаьржанаш

ПРИМЕР ВЫХОДА:
чёрный — ӏаьржа
• чёрное платье → ӏаьржа коч
• чёрный хлеб → ӏаьржа бепиг
• чёрная работа → ӏаьржа болх

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

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

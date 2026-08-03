package ai

import (
	"chetoru/internal/models"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// articleSystemPrompt asks for structure and nothing else. The retired card
// formatter asked this same model for a finished layout, which is why the bot
// ended up with two: whatever the model returns here is data, and pkg/tools
// stays the only thing that decides what a card looks like.
const articleSystemPrompt = `Ты разбираешь статьи русско-чеченского словаря на части.

На входе — русское заглавное слово и текст статьи одной строкой. Верни ТОЛЬКО JSON:

{"senses":[{"gloss":"чеченский перевод","note":"русское уточнение или пусто"}],
 "examples":[{"ce":"чеченская фраза","ru":"русская фраза"}]}

Правила:
1. senses — чеченские эквиваленты в том же порядке, что в статье, по одному на
   каждый пронумерованный смысл. gloss содержит ТОЛЬКО чеченский текст.
2. Русские пояснения при смысле («учреждение», «почерк», «перен.», «разг.»)
   идут в note, а не в gloss. Грамматические пометы («м», «ж», «несов.»,
   «кого-что») выбрасывай — это не перевод.
3. examples — примеры употребления. ce — чеченская сторона, ru — русская.
   В источнике русская сторона идёт первой, а разделитель — тире.
4. Тильда (~) заменяет заглавное слово. Раскрой её в правильной форме:
   «~ отдыха» под «Дом» → «дом отдыха», «~ки» под «Силок» → «силки».
   Это единственное место, где нужно твоё знание русской морфологии.
5. Ничего не выдумывай и ничего не переводи сам. Если часть статьи не
   разбирается — пропусти её. Пустой список лучше выдуманного содержимого.
6. Никакого форматирования: ни звёздочек, ни HTML, ни markdown. Только JSON.`

// StructureArticle breaks one Russian–Chechen article into senses and examples.
func (c *Client) StructureArticle(ctx context.Context, headword, article string) (*models.ArticleStructure, error) {
	raw, err := c.complete(ctx, []message{
		{Role: "system", Content: articleSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Заглавное слово: %s\nСтатья: %s", headword, article)},
	}, 1500)
	if err != nil {
		return nil, err
	}

	var out models.ArticleStructure
	if err := json.Unmarshal([]byte(trimJSONFence(raw)), &out); err != nil {
		return nil, fmt.Errorf("parse model output: %w", err)
	}

	// A model that returned nothing usable must not overwrite the regex parser's
	// answer with an empty one, so that reads as a failure rather than a result.
	if len(out.Senses) == 0 && len(out.Examples) == 0 {
		return nil, fmt.Errorf("model returned no structure")
	}
	return &out, nil
}

// trimJSONFence strips a ```json fence some models wrap the answer in.
func trimJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i != -1 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

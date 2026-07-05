package tools

import (
	"fmt"
	"strings"
)

const maxCardExamples = 5

func formatDictionaryCard(text, originalLang, translateLang string) string {
	first, rest, hasRest := strings.Cut(text, "\n")
	original, translate, ok := strings.Cut(first, " — ")
	if !ok {
		return text
	}

	card := newTranslationCard(original, originalLang, translateLang)
	card.Translations = append(card.Translations, translate)

	if hasRest {
		for _, line := range strings.Split(rest, "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "•"))
			line = strings.TrimSpace(expandCardAbbreviations(line))
			if line == "" {
				continue
			}
			if strings.Contains(line, "→") {
				card.Examples = append(card.Examples, line)
				continue
			}
			card.Translations = append(card.Translations, line)
		}
	}
	return card.Render()
}

func formatSimpleCard(original, translate, originalLang, translateLang string) string {
	card := newTranslationCard(original, originalLang, translateLang)
	card.Translations = append(card.Translations, translate)
	return card.Render()
}

type translationCard struct {
	Headword        string
	OriginalLang    string
	TranslateLang   string
	Translations    []string
	Examples        []string
	OmittedExamples int
}

func newTranslationCard(headword, originalLang, translateLang string) translationCard {
	return translationCard{
		Headword:      headword,
		OriginalLang:  originalLang,
		TranslateLang: translateLang,
	}
}

func (c translationCard) Render() string {
	if c.Headword == "" {
		return strings.Join(c.Translations, "\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📘 <b>%s</b>", c.Headword)
	if direction := directionLabel(c.OriginalLang, c.TranslateLang); direction != "" {
		fmt.Fprintf(&b, "\n<i>%s</i>", direction)
	}

	if len(c.Translations) > 0 {
		b.WriteString("\n\n<b>")
		if len(c.Translations) == 1 {
			b.WriteString("Перевод")
		} else {
			b.WriteString("Переводы")
		}
		b.WriteString("</b>\n")
		for _, translation := range uniqueNonEmpty(c.Translations) {
			fmt.Fprintf(&b, "• %s\n", translation)
		}
	}

	examples := uniqueNonEmpty(c.Examples)
	if len(examples) > maxCardExamples {
		c.OmittedExamples = len(examples) - maxCardExamples
		examples = examples[:maxCardExamples]
	}
	if len(examples) > 0 {
		b.WriteString("\n<b>Примеры</b>\n")
		for _, example := range examples {
			fmt.Fprintf(&b, "• %s\n", example)
		}
		if c.OmittedExamples > 0 {
			fmt.Fprintf(&b, "\n<i>Еще %d прим. — через кнопку «Еще»</i>\n", c.OmittedExamples)
		}
	}

	return strings.TrimSpace(b.String())
}

func directionLabel(originalLang, translateLang string) string {
	switch strings.ToUpper(strings.TrimSpace(originalLang)) + ">" + strings.ToUpper(strings.TrimSpace(translateLang)) {
	case "RUS>CHE":
		return "с русского на чеченский"
	case "CHE>RUS":
		return "с чеченского на русский"
	default:
		return ""
	}
}

func expandCardAbbreviations(s string) string {
	replacer := strings.NewReplacer(
		"с.-х.", "сельскохозяйственное",
		"мн.", "множественное",
	)
	return replacer.Replace(s)
}

func uniqueNonEmpty(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

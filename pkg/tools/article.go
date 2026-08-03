package tools

import (
	"chetoru/internal/models"
	"encoding/json"
	"strings"
)

// ParseArticle splits a Russian–Chechen article into Chechen glosses and
// examples:
//
//	м 1) цӏа; деревянный ~- дечиган цӏа 2) (учреждение) цӏа; ~ отдыха - садаӏаран цӏа
//
// Measured over 1097 live entries, every tilde and every "1)" sits in this one
// corpus; the other three arrive atomized and reach the card untouched.
func ParseArticle(head, body string) (glosses []string, examples []example) {
	body = boldRe.ReplaceAllString(body, "")
	body = stripLabels(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), "-")))

	for _, part := range meaningRe.Split(body, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Labels head each sense too ("2. несов. дала"), and stripping once
		// left the first standing in as a meaning.
		part = stripLabels(part)

		main, rest := part, ""
		if i := findMainSemicolon(part); i != -1 {
			main, rest = part[:i], part[i+1:]
		}

		if gloss := expandAbbreviations(cleanTranslation(main)); gloss != "" {
			if expanded, exact := replaceTildeWithWord(gloss, head); exact {
				glosses = append(glosses, expanded)
			}
		}
		examples = append(examples, articleExamples(rest, head)...)
	}
	return glosses, examples
}

// articleExamples reads a sense's semicolon-separated example list. The source
// writes them Russian first, the card shows Chechen first, so the sides swap.
func articleExamples(text, head string) []example {
	var out []example
	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		expanded, exact := replaceTildeWithWord(part, head)
		// A tilde wordStem refuses to guess would build a word that does not
		// exist; no illustration beats that.
		if !exact {
			continue
		}
		russian, chechen, ok := splitExample(expanded)
		if !ok || russian == "" || chechen == "" {
			continue
		}
		out = append(out, example{
			chechen: expandAbbreviations(chechen),
			russian: expandAbbreviations(russian),
		})
	}
	return out
}

// stripLabels peels the grammar metadata heading an article or a sense —
// gender, adjective endings, aspect and government — until nothing is left.
// Without it «Дома» read as "нареч. цӏахь" and «Идти» opened with "несов.".
func stripLabels(text string) string {
	for {
		before := text
		text = strings.TrimSpace(endingsRe.ReplaceAllString(text, ""))
		text = strings.TrimSpace(grammarRe.ReplaceAllString(text, ""))
		text = strings.TrimSpace(verbLabelRe.ReplaceAllString(text, ""))
		if text == before {
			return text
		}
	}
}

// articleParts prefers the structure cmd/parse_articles wrote at ingest and
// falls back to ParseArticle. The model reads what the regex cannot: a gloss
// that carries its example without a semicolon, and a tilde under a stem
// Russian spelling does not determine. Only the fill changes — the card's shape
// is the same through either door.
func articleParts(p models.TranslationPairs, head, body string) ([]string, []example) {
	if p.Structured == "" {
		return ParseArticle(head, body)
	}
	var st models.ArticleStructure
	if err := json.Unmarshal([]byte(p.Structured), &st); err != nil {
		return ParseArticle(head, body)
	}

	glosses := make([]string, 0, len(st.Senses))
	for _, s := range st.Senses {
		if s.Gloss == "" {
			continue
		}
		// The card's own convention for a qualifier: leading parentheses, which
		// splitQualifiers lifts back out of the bold at render time.
		if s.Note != "" {
			glosses = append(glosses, "("+s.Note+") "+s.Gloss)
			continue
		}
		glosses = append(glosses, s.Gloss)
	}

	examples := make([]example, 0, len(st.Examples))
	for _, ex := range st.Examples {
		if ex.Chechen == "" || ex.Russian == "" {
			continue
		}
		examples = append(examples, example{chechen: ex.Chechen, russian: ex.Russian})
	}
	return glosses, examples
}

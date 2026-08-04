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

		// A sense whose own first clause carries an example separator is not a
		// gloss at all: «Трезвонит» stores "телефон - ӏуьйранна дуьйна телефон
		// ека" as its whole first sense, and that string became a headword.
		if _, _, ok := splitExample(main); ok {
			rest = strings.TrimSpace(main + ";" + rest)
			main = ""
		}

		// stripLabels again after the tail cut: what the cut leaves behind starts
		// mid-label. «Спасибо» stores "2. в знач. сказ., кому баркалла ду", and
		// cutting at the last period yields ", кому баркалла ду" — a comma, a case
		// marker, and only then the word.
		// Abbreviations are expanded last, and only in what survives. Expanding
		// first spends the periods dropLabelTail reads: «Камень» opens "м, тж.
		// собир. тӏулг", and once that became "м, также (собирательное) тӏулг"
		// there was no period left to cut at and no pattern to match, so the
		// first Chechen word offered for «камень» was a gender marker.
		if gloss := expandAbbreviations(stripLabels(dropLabelTail(cleanTranslation(main)))); gloss != "" {
			if expanded, exact := replaceTildeWithWord(gloss, head); exact {
				glosses = append(glosses, expanded)
			}
		}
		examples = append(examples, articleExamples(rest, head)...)
	}
	return glosses, examples
}

// dropLabelTail cuts a gloss back to its last period. Every abbreviation the
// card knows is expanded by the time this runs, so a surviving period is a
// grammar label stripLabels has no pattern for — «Телефонировать» opens "сов. и
// несов., что, о чём и без доп. телефон тоха, телефон етта", and chasing that
// vocabulary one abbreviation at a time is a race with a paper dictionary.
// The real translation is always what follows the last one.
func dropLabelTail(gloss string) string {
	i := lastPeriodOutsideParens(gloss)
	if i == -1 {
		return gloss
	}
	// A period inside the last few characters is punctuation, not a label, and
	// cutting there would leave nothing.
	rest := strings.TrimSpace(gloss[i+1:])
	if rest == "" {
		return gloss
	}
	return rest
}

// lastPeriodOutsideParens finds the label period to cut at, ignoring the ones
// inside a qualifier: «Мать» stores "ж (род. матери) нана", and cutting at the
// period in «род.» left the card offering «матери) нана» as Chechen.
func lastPeriodOutsideParens(s string) int {
	depth, at := 0, -1
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 {
				at = i
			}
		}
	}
	return at
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
		// A label the previous pass cut in half leaves its punctuation behind, and
		// every pattern below is anchored, so the comma alone stops the loop dead.
		text = strings.TrimSpace(strings.TrimLeft(text, ",;:-— "))
		text = strings.TrimSpace(endingsRe.ReplaceAllString(text, ""))
		text = strings.TrimSpace(crossRefRe.ReplaceAllString(text, ""))
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
		// The model reads the article well but hands back the odd sense with its
		// own bookkeeping still attached — «Лев» came out as "2 м лев
		// (Болгарера ахча)", homonym number and gender and all, under a card
		// that numbers its senses itself. Same peeling the regex path gets.
		gloss := stripLabels(senseNumRe.ReplaceAllString(strings.TrimSpace(s.Gloss), ""))
		if gloss == "" {
			continue
		}
		// The card's own convention for a qualifier: leading parentheses, which
		// splitQualifiers lifts back out of the bold at render time.
		if s.Note != "" {
			glosses = append(glosses, "("+s.Note+") "+gloss)
			continue
		}
		glosses = append(glosses, gloss)
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

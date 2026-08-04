// Rendering to Telegram HTML.
package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
)

func EscapeUnclosedTags(text string) string {
	// No angle brackets means no tags to balance.
	if !strings.ContainsAny(text, "<>") {
		return text
	}
	// A stray bracket ("цӏа < дитт") survives tag matching but breaks
	// Telegram's HTML parser, which rejects the whole message.
	if strings.Count(text, "<") != strings.Count(text, ">") {
		text = Clean(text)
		text = strings.ReplaceAll(text, "<", "")
		return strings.ReplaceAll(text, ">", "")
	}
	matches := tagRe.FindAllString(text, -1)
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

// exampleLine renders a usage example with the studied language first. Every
// card that shows an example goes through here or FormatExample, so the
// translation card and the grammar card can no longer disagree on which
// language leads.
func exampleLine(chechen, russian string) string {
	return chechen + " → " + russian
}

// FormatExample renders a usage example for a Telegram card. Italic is the
// card's mark for an example and means nothing else.
func FormatExample(chechen, russian string) string {
	return "<i>" + exampleLine(chechen, russian) + "</i>"
}

// FormatTranslationLite formats a dictionary entry into a lightweight, consistent style.
// originalWord is used for tilde replacement (~ое -> чёрное).
//
// boldGloss says the Chechen side is the gloss, not the headword — Russian
// entries carry their Chechen translation in the senses. Bold always marks the
// studied language, so it moves with it; with no language known the headword
// takes it, matching /random and /wotd.
func FormatTranslationLite(text string, originalWord string, boldGloss bool) string {
	if text == "" {
		return ""
	}

	word := strings.TrimSpace(originalWord)

	// Strip the bolded headword; we render it ourselves from originalWord.
	text = boldRe.ReplaceAllString(text, "")

	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimSpace(text)

	text = endingsRe.ReplaceAllString(text, "")
	// Gender marker first ("с нескл. амплуа"), then the label loop.
	text = grammarRe.ReplaceAllString(text, "")
	for {
		stripped := verbLabelRe.ReplaceAllString(text, "")
		if stripped == text {
			break
		}
		text = stripped
	}

	type sense struct {
		main     string
		examples []string
	}
	var senses []sense

	for _, part := range meaningRe.Split(text, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		semicolonIndex := findMainSemicolon(part)
		main := part
		var examples []string

		if semicolonIndex != -1 {
			main = part[:semicolonIndex]
			examples = parseExamples(part[semicolonIndex+1:], !boldGloss)
		}

		main = expandAbbreviations(cleanTranslation(main))
		if word != "" {
			main, _ = replaceTildeWithWord(main, word)
			// An example whose tilde needs a stem we cannot derive is dropped
			// rather than shown with a guess: it is an illustration, and no
			// illustration beats one built on a word that does not exist.
			kept := examples[:0]
			for _, example := range examples {
				if expanded, ok := replaceTildeWithWord(example, word); ok {
					kept = append(kept, expanded)
				}
			}
			examples = kept
		}
		for i, example := range examples {
			examples[i] = expandAbbreviations(example)
		}

		// A marker with no translation of its own ("2. ; пример - масал") is a
		// carrier for its examples, not a sense the reader should count.
		if main == "" && len(senses) > 0 {
			last := &senses[len(senses)-1]
			last.examples = append(last.examples, examples...)
			continue
		}
		senses = append(senses, sense{main: main, examples: examples})
	}

	bold := func(s string) string {
		if s == "" {
			return s
		}
		return "<b>" + s + "</b>"
	}
	header := word
	if !boldGloss {
		header = bold(header)
	}
	gloss := func(s string) string {
		if boldGloss {
			return bold(s)
		}
		return s
	}

	// parseExamples caps each sense at five, which is fine until a word has
	// twenty-six of them: «Идти» ran to fifty-eight lines, well past what anyone
	// scrolls. The budget is spent in sense order, so the examples that survive
	// are the ones under the senses the reader meets first.
	budget := maxCardExamples
	take := func(examples []string) []string {
		// Per sense as well as per card: spending it greedily let «Идти» give all
		// six to sense one — which spends them on parenthetical fragments — and
		// nothing to the twenty-one senses under it.
		limit := budget
		if len(senses) > 1 && limit > maxExamplesPerSense {
			limit = maxExamplesPerSense
		}
		if len(examples) > limit {
			examples = examples[:limit]
		}
		budget -= len(examples)
		return examples
	}
	// The cut is silent: there is no second page to send a count to.
	render := func(lines []string) string {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var lines []string
	// One sense stays a one-liner — the common case, and a numbered list of one
	// is noise. Two or more put the headword on its own line so the numbers
	// start at 1 under it.
	if len(senses) == 1 && senses[0].main != "" {
		switch {
		case word != "":
			lines = append(lines, header+" — "+gloss(senses[0].main))
		default:
			lines = append(lines, gloss(senses[0].main))
		}
		return render(append(lines, renderExamples(take(senses[0].examples))...))
	}

	if word != "" {
		lines = append(lines, header)
	}
	number := 0
	for _, s := range senses {
		if s.main != "" {
			number++
			lines = append(lines, fmt.Sprintf("%d. %s", number, gloss(s.main)))
		}
		lines = append(lines, renderExamples(take(s.examples))...)
	}

	return render(lines)
}

// renderExamples prefixes a sense's examples. A lone example sits flush under
// its sense; three or more get an indent (Telegram keeps leading spaces) so a
// long list reads as belonging to the sense above it rather than to the card.
func renderExamples(examples []string) []string {
	prefix := "• "
	if len(examples) >= 3 {
		prefix = "   • "
	}
	out := make([]string, 0, len(examples))
	for _, example := range examples {
		out = append(out, prefix+example)
	}
	return out
}

const (
	// maxCardExamples caps how many usage examples one card may carry, across
	// all its senses; maxExamplesPerSense keeps one sense from taking them all.
	maxCardExamples     = 6
	maxExamplesPerSense = 2
)

// FirstExampleFor returns the leading usage example a card would show for the
// query, as plain text — callers escape it before wrapping.
//
// /wotd and /random used to mine the examples themselves, with their own parser
// and their own five-pair budget. That budget was spent on the academic
// corpus's plain senses, which carry no examples at all, so the article that
// did carry them was never reached and the daily word shipped without the one
// thing a learner needs most. Now there is one parser and one answer: whatever
// the card would show first.
func FirstExampleFor(query string, pairs []models.TranslationPairs) (string, bool) {
	for _, b := range collect(query, pairs).blocks {
		if len(b.examples) > 0 {
			return exampleLine(b.examples[0].chechen, b.examples[0].russian), true
		}
	}
	return "", false
}

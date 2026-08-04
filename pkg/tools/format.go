// Rendering to Telegram HTML.
package tools

import (
	"chetoru/internal/models"
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

package tools

import (
	"chetoru/internal/models"
	"strings"
)

// FormatPairs renders a bare list of pairs, one per line. It is the fallback
// for the two places that have no single headword to build a card around: the
// prefix suggestions offered after a miss, and a result set where FormatCard
// could not place anything.
func FormatPairs(pairs []models.TranslationPairs) string {
	lines := make([]string, 0, len(pairs))
	for _, p := range pairs {
		original := EscapeUnclosedTags(strings.TrimSpace(p.Original))
		translate := Clean(EscapeUnclosedTags(strings.TrimSpace(p.Translate)))
		if original == "" || translate == "" {
			continue
		}
		// Bold marks Chechen here exactly as it does on the card.
		if p.TranslateLang == "CHE" && p.OriginalLang != "CHE" {
			lines = append(lines, original+" — <b>"+translate+"</b>")
		} else {
			lines = append(lines, "<b>"+original+"</b> — "+translate)
		}
	}
	return strings.Join(lines, "\n")
}

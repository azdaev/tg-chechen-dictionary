package tools

import (
	"chetoru/internal/models"
	"strings"
)

// FormatSuggestions renders near-misses one per line: the headword and its
// leading meaning, Chechen bold, the way the card marks it.
//
// The pairs come straight out of the prefix lookup, and the Russian–Chechen
// corpus packs a whole entry into one string — so dumping them raw offered
// «Карандаш — м къолам; химический ~ - шекъа долун къолам; цветные ~и -
// бес-бесара къоламаш», with the article's own Russian wrapped in the bold that
// is supposed to mean Chechen.
func FormatSuggestions(pairs []models.TranslationPairs) string {
	lines := make([]string, 0, len(pairs))
	for _, p := range pairs {
		line := suggestionLine(p)
		if line == "" {
			// A pair the card cannot place still beats showing nothing.
			line = FormatPairs([]models.TranslationPairs{p})
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func suggestionLine(p models.TranslationPairs) string {
	c := collect(Clean(p.Original), []models.TranslationPairs{p})
	if len(c.blocks) == 0 || len(c.blocks[0].senses) == 0 {
		return ""
	}
	b := c.blocks[0]
	name, gloss := headCase(b.head), firstVariant(b.senses[0])
	if name == "" || gloss == "" {
		return ""
	}
	if b.cheHead {
		return "<b>" + name + "</b> — " + gloss
	}
	return name + " — <b>" + gloss + "</b>"
}

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

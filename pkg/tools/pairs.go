package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
)

// FormatPairs renders translation pairs into the bot's display text. Each pair
// is shown via its stored AI formatting, the lightweight dictionary formatter
// (for entries with numbered senses or tilde placeholders), or a plain
// "original — translation" line. The joined result is trimmed.
//
// It is the single source of truth for translation formatting, shared by the
// net handlers (HandleText, HandleMoreTranslations).
func FormatPairs(pairs []models.TranslationPairs) string {
	var b strings.Builder
	for _, t := range pairs {
		b.WriteString(formatPair(t))
	}
	return strings.TrimSpace(b.String())
}

// formatPair renders a single pair, including the trailing blank-line separator.
func formatPair(t models.TranslationPairs) string {
	if t.FormattedChosen == "ai" && t.FormattedAI != "" {
		return t.FormattedAI + "\n\n"
	}
	// Locally stored pairs carry raw content; tame brackets here so a stray
	// "<" can't break the HTML-mode send. (AI formatting above is trusted —
	// it intentionally emits Telegram HTML.)
	t.Original = EscapeUnclosedTags(t.Original)
	t.Translate = EscapeUnclosedTags(t.Translate)
	// A dictionary-structured side becomes the gloss; the other side is its
	// headword. Telegram-side checks Translate first to match historical output.
	if hasDictionaryMarkup(t.Translate) {
		entry := fmt.Sprintf("**%s** - %s", t.Original, t.Translate)
		return formatDictionaryCard(FormatTranslationLite(entry, t.Original), t.OriginalLang, t.TranslateLang) + "\n\n"
	}
	if hasDictionaryMarkup(t.Original) {
		entry := fmt.Sprintf("**%s** - %s", t.Translate, t.Original)
		return formatDictionaryCard(FormatTranslationLite(entry, t.Translate), t.TranslateLang, t.OriginalLang) + "\n\n"
	}
	return formatSimpleCard(t.Original, Clean(t.Translate), t.OriginalLang, t.TranslateLang) + "\n\n"
}

// hasDictionaryMarkup reports whether a string carries dictionary structure:
// numbered senses ("1)", "2)") or a tilde headword placeholder ("~").
func hasDictionaryMarkup(s string) bool {
	return strings.Contains(s, "1)") || strings.Contains(s, "2)") || strings.Contains(s, "~")
}

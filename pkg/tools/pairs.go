package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
)

// FormatPairs renders translation pairs into the bot's display text. Each pair
// is shown via its stored AI formatting, the lightweight dictionary formatter
// (for entries with numbered senses or tilde placeholders), or a plain
// "original — translation" line. Neighbours sharing a headword merge into one
// numbered block — ranking already made them adjacent, and four blocks opening
// with the same word is the user's own query read back to them. The joined
// result is trimmed.
//
// It is the single source of truth for translation formatting, shared by the
// net handlers (HandleText, HandleMoreTranslations).
func FormatPairs(pairs []models.TranslationPairs) string {
	var b strings.Builder
	for i := 0; i < len(pairs); {
		j := i + 1
		for j < len(pairs) && sameHeadword(pairs[i], pairs[j]) {
			j++
		}
		b.WriteString(formatBlock(pairs[i:j]))
		i = j
	}
	return strings.TrimSpace(b.String())
}

// sameHeadword reports whether two pairs can share one headword block. Only
// plain pairs qualify: an AI rendering and a structured gloss each bring their
// own headword line, and merging those would mean re-parsing what they already
// decided.
func sameHeadword(a, b models.TranslationPairs) bool {
	return plainPair(a) && plainPair(b) &&
		NormalizeSearch(a.Original) == NormalizeSearch(b.Original) &&
		a.OriginalLang == b.OriginalLang && a.TranslateLang == b.TranslateLang
}

func plainPair(t models.TranslationPairs) bool {
	return !hasAIFormatting(t) && !hasDictionaryMarkup(t.Original) && !hasDictionaryMarkup(t.Translate)
}

func hasAIFormatting(t models.TranslationPairs) bool {
	return t.FormattedChosen == "ai" && t.FormattedAI != ""
}

// formatBlock renders one headword's worth of pairs, including the trailing
// blank-line separator.
func formatBlock(group []models.TranslationPairs) string {
	if len(group) == 1 {
		return formatPair(group[0])
	}

	head := EscapeUnclosedTags(group[0].Original)
	boldGloss := glossIsChechen(group[0])
	var b strings.Builder
	if boldGloss {
		b.WriteString(head + "\n")
	} else {
		b.WriteString("<b>" + head + "</b>\n")
	}
	for i, t := range group {
		gloss := Clean(EscapeUnclosedTags(t.Translate))
		if boldGloss {
			gloss = "<b>" + gloss + "</b>"
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, gloss)
	}
	b.WriteString("\n")
	return b.String()
}

// formatPair renders a single pair, including the trailing blank-line separator.
func formatPair(t models.TranslationPairs) string {
	if hasAIFormatting(t) {
		return t.FormattedAI + "\n\n"
	}
	// Locally stored pairs carry raw content; tame brackets here so a stray
	// "<" can't break the HTML-mode send. (AI formatting above is trusted —
	// it intentionally emits Telegram HTML.)
	t.Original = EscapeUnclosedTags(t.Original)
	t.Translate = EscapeUnclosedTags(t.Translate)

	// A dictionary-structured side becomes the gloss; the other side is its
	// headword. Telegram-side checks Translate first to match historical output.
	// The languages ride along with the swap — bold follows the Chechen side,
	// not a fixed field.
	if !hasDictionaryMarkup(t.Translate) && hasDictionaryMarkup(t.Original) {
		t.Original, t.Translate = t.Translate, t.Original
		t.OriginalLang, t.TranslateLang = t.TranslateLang, t.OriginalLang
	}
	boldGloss := glossIsChechen(t)

	if hasDictionaryMarkup(t.Translate) {
		entry := fmt.Sprintf("**%s** - %s", t.Original, t.Translate)
		return FormatTranslationLite(entry, t.Original, boldGloss) + "\n\n"
	}
	if boldGloss {
		return fmt.Sprintf("%s — <b>%s</b>\n\n", t.Original, Clean(t.Translate))
	}
	return fmt.Sprintf("<b>%s</b> — %s\n\n", t.Original, Clean(t.Translate))
}

// glossIsChechen reports whether the studied language sits on the gloss side,
// which is where the bold goes. Pairs with no language recorded (a legacy cache
// entry, a prefix suggestion) fall back to bolding the headword.
func glossIsChechen(t models.TranslationPairs) bool {
	return t.TranslateLang == "CHE" && t.OriginalLang != "CHE"
}

// hasDictionaryMarkup reports whether a string carries dictionary structure:
// numbered senses ("1)", "2)") or a tilde headword placeholder ("~").
func hasDictionaryMarkup(s string) bool {
	return strings.Contains(s, "1)") || strings.Contains(s, "2)") || strings.Contains(s, "~")
}

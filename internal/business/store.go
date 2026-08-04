// The write path: persisting a pair dosham answered with, and formatting it
// with the LLM afterwards. All of it detached from the request.
package business

import (
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// storeTranslationPair persists one pair and reports whether it was new. mayFormat
// says the caller still has LLM budget for it; a pair stored without one keeps
// its place in the moderation table, it just arrives there without a suggestion.
func (b *Business) storeTranslationPair(entry models.Entry, translation models.Translation, mayFormat bool) (inserted bool) {
	if b.dictRepo == nil {
		return false
	}

	originalLang := inferOriginalLang(translation.LanguageCode)
	if originalLang == "" {
		return
	}

	pair := repository.TranslationPair{
		OriginalRaw:         strings.TrimSpace(entry.Content),
		OriginalClean:       normalizeText(entry.Content),
		OriginalLang:        originalLang,
		TranslationRaw:      strings.TrimSpace(translation.Content),
		TranslationClean:    normalizeText(translation.Content),
		TranslationLang:     translation.LanguageCode,
		Source:              "api",
		SourceEntryID:       toNullString(entry.EntryID),
		SourceTranslationID: toNullString(translation.TranslationID),
		Rate:                entry.Rate,
		EntryType:           entry.Type,
		Subtype:             entry.Subtype,
		EntryIndex:          entry.EntryIndex,
		EntryNotes:          entry.Notes,
	}
	if pair.OriginalClean == "" || pair.TranslationClean == "" {
		return false
	}

	pairID, inserted, err := b.dictRepo.InsertTranslationPair(context.Background(), pair)
	if err != nil {
		b.log.Printf("failed to insert dictionary pair: %v\n", err)
		return false
	}
	// Duplicates already went through formatting and moderation when first
	// stored; re-running them would burn AI calls and overwrite the result.
	if !inserted || pairID == 0 {
		return false
	}
	if !mayFormat {
		return true
	}

	if b.aiFormattingEnabled.Load() && b.aiClient != nil {
		b.bg.Go(func() { b.formatPairWithAI(pairID, pair.OriginalClean, pair.OriginalRaw, pair.TranslationRaw) })
	} else if b.onPairReady != nil {
		// No AI client — trigger moderation immediately
		b.bg.Go(func() { b.onPairReady(pairID, pair.OriginalClean) })
	}
	return true
}

// formatPairWithAI asynchronously formats a dictionary pair using AI, saves it, then triggers moderation.
func (b *Business) formatPairWithAI(pairID int64, cleanWord, originalRaw, translationRaw string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build raw entry for formatting
	rawEntry := fmt.Sprintf("**%s** - %s", originalRaw, translationRaw)

	// Format with AI
	formatted, err := b.aiClient.FormatDictionaryEntry(ctx, rawEntry)
	if err != nil {
		b.log.Printf("ai formatting failed for pair %d: %v\n", pairID, err)
	} else {
		// Save to database
		if err := b.dictRepo.UpdateTranslationPairFormatting(ctx, pairID, formatted, ""); err != nil {
			b.log.Printf("failed to save ai formatting for pair %d: %v\n", pairID, err)
		} else {
			b.log.Printf("successfully formatted pair %d with AI\n", pairID)
		}
	}

	// Trigger moderation after AI formatting (or failed attempt)
	if b.onPairReady != nil {
		b.onPairReady(pairID, cleanWord)
	}
}

// saveWordForms records a paradigm off the request path. A failure costs the
// next reader one more miss, which is what happens today anyway.
func (b *Business) saveWordForms(headword string, forms []string) {
	if b.dictRepo == nil || headword == "" || len(forms) == 0 {
		return
	}
	b.bg.Go(func() {
		if err := b.dictRepo.SaveWordForms(context.Background(), headword, forms); err != nil {
			b.log.Printf("failed to save word forms for %q: %v\n", headword, err)
		}
	})
}

func toNullString(v string) sql.NullString {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func normalizeText(text string) string {
	return tools.NormalizeSearch(text)
}

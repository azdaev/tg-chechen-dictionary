// The write path: persisting what dosham answered with, detached from the
// request.
package business

import (
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"database/sql"
	"strings"
)

// storeTranslationPair persists one pair; a duplicate is a no-op.
func (b *Business) storeTranslationPair(entry models.Entry, translation models.Translation) {
	if b.dictRepo == nil {
		return
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
		return
	}
	if _, _, err := b.dictRepo.InsertTranslationPair(context.Background(), pair); err != nil {
		b.log.Printf("failed to insert dictionary pair: %v\n", err)
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

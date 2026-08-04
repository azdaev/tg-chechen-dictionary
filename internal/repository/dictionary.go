// Dictionary lookups: the exact key, the folded key, prefixes and counts.
package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FindTranslationPairs returns stored pairs for a normalized word. The order by
// decides which rows survive the limit, not what the user finally sees —
// business.rankAndDedup re-sorts the result with a total order. Both layers
// apply the same preference (moderated, then dosham's rate, then the shortest
// translation) so the truncated set and the displayed set agree; change one and
// change the other.
// pairKey names the column pair a lookup matches on: the exact clean spelling,
// or the folded one that survives a palochka or a long-vowel mark the user
// could not type.
type pairKey string

const (
	keyClean  pairKey = "clean"
	keyFolded pairKey = "folded"
)

// findPairs returns stored pairs whose original or translation side matches key,
// with the matched side leading. Deleted pairs are excluded, moderated ones sort
// first, then the source dictionary's rate.
func (r *Repository) findPairs(ctx context.Context, col pairKey, key string, limit int) ([]models.TranslationPairs, error) {
	if key == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}

	rows, err := r.db.QueryContext(
		ctx,
		fmt.Sprintf(`select
			original_raw,
			original_%[1]s,
			original_lang,
			translation_raw,
			translation_lang,
			formatted_ai,
			formatted_chosen,
			rate,
			entry_type,
			subtype,
			entry_index,
			entry_notes,
			structured_json
		from dictionary_pairs
		where (formatted_chosen is null or formatted_chosen != 'deleted')
		  and (original_%[1]s = ? or translation_%[1]s = ?)
		order by (formatted_chosen is null), rate desc, length(translation_raw), id
		limit ?;`, col),
		key, key, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.TranslationPairs, 0, limit)
	for rows.Next() {
		var originalRaw, originalLang, translationRaw, translationLang string
		var originalKey, formattedAI, formattedChosen, entryType, entryNotes, structured sql.NullString
		var rate, subtype, entryIndex int
		if err := rows.Scan(&originalRaw, &originalKey, &originalLang, &translationRaw, &translationLang, &formattedAI, &formattedChosen, &rate, &entryType, &subtype, &entryIndex, &entryNotes, &structured); err != nil {
			return nil, err
		}

		// Subtype, EntryIndex and Notes describe dosham's entry headword, which
		// for every corpus that fills them in is the Chechen side. They ride
		// along unchanged through the reverse swap below for exactly that
		// reason: the swap moves which side leads, not which side is Chechen.
		pair := models.TranslationPairs{
			Original:        originalRaw,
			Translate:       translationRaw,
			OriginalLang:    originalLang,
			TranslateLang:   translationLang,
			FormattedAI:     formattedAI.String,
			FormattedChosen: formattedChosen.String,
			Rate:            rate,
			// Read from the stored direction, before the swap below moves it.
			Packed:     translationLang == "CHE",
			EntryType:  entryType.String,
			Subtype:    subtype,
			EntryIndex: entryIndex,
			Notes:      entryNotes.String,
			Structured: structured.String,
		}

		if originalKey.String != key {
			// Reverse hit: the matched side leads, so the languages swap with it.
			pair.Original, pair.Translate = pair.Translate, pair.Original
			pair.OriginalLang, pair.TranslateLang = pair.TranslateLang, pair.OriginalLang
		}
		results = append(results, pair)
	}

	return results, rows.Err()
}

func (r *Repository) FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error) {
	return r.findPairs(ctx, keyClean, cleanWord, limit)
}

// FindTranslationPairsByFolded mirrors FindTranslationPairs but matches on the
// spelling-insensitive columns, so a query that dropped a palochka or a long
// vowel mark still reaches a word we already stored. No new ranking is
// introduced: the caller runs the result through the same rankAndDedup as the
// exact lookup.
func (r *Repository) FindTranslationPairsByFolded(ctx context.Context, folded string, limit int) ([]models.TranslationPairs, error) {
	return r.findPairs(ctx, keyFolded, folded, limit)
}

// FindTranslationPairsByPrefix returns pairs where either side starts with
// prefix, shortest matches first (closest to a lemma). It backs suggestions
// for failed searches: the local table holds lemmas ("яблоко") that dosham's
// substring search cannot reach from an inflected query ("яблоками"). Range
// comparisons keep the clean-word indexes usable, unlike LIKE.
func (r *Repository) FindTranslationPairsByPrefix(ctx context.Context, prefix string, limit int) ([]models.TranslationPairs, error) {
	if prefix == "" || limit <= 0 {
		return nil, nil
	}
	hi := prefix + "\uffff"
	rows, err := r.db.QueryContext(
		ctx,
		`select
			original_raw,
			original_clean,
			original_lang,
			translation_raw,
			translation_clean,
			translation_lang,
			formatted_ai,
			formatted_chosen
		from dictionary_pairs
		where (formatted_chosen is null or formatted_chosen != 'deleted')
		  and ((original_clean >= ? and original_clean < ?) or (translation_clean >= ? and translation_clean < ?))
		order by min(length(original_clean), length(translation_clean))
		limit ?;`,
		prefix, hi, prefix, hi, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.TranslationPairs, 0, limit)
	for rows.Next() {
		var originalRaw, originalClean, originalLang, translationRaw, translationClean, translationLang string
		var formattedAI, formattedChosen sql.NullString
		if err := rows.Scan(&originalRaw, &originalClean, &originalLang, &translationRaw, &translationClean, &translationLang, &formattedAI, &formattedChosen); err != nil {
			return nil, err
		}

		pair := models.TranslationPairs{
			Original:        originalRaw,
			Translate:       translationRaw,
			OriginalLang:    originalLang,
			TranslateLang:   translationLang,
			Packed:          translationLang == "CHE",
			FormattedAI:     formattedAI.String,
			FormattedChosen: formattedChosen.String,
		}
		if !strings.HasPrefix(originalClean, prefix) {
			pair.Original, pair.Translate = pair.Translate, pair.Original
			pair.OriginalLang, pair.TranslateLang = pair.TranslateLang, pair.OriginalLang
		}
		results = append(results, pair)
	}

	return results, rows.Err()
}

// FindStrictlyApprovedPairs returns only pairs that have been explicitly moderated (formatted_chosen is not null and not 'deleted')
func (r *Repository) FindStrictlyApprovedPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := r.db.QueryContext(
		ctx,
		`select
			original_raw,
			original_clean,
			translation_raw,
			translation_clean
		from dictionary_pairs
		where formatted_chosen is not null and formatted_chosen != 'deleted' and (original_clean = ? or translation_clean = ?)
		limit ?;`,
		cleanWord, cleanWord, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]models.TranslationPairs, 0, limit)
	for rows.Next() {
		var originalRaw, originalClean, translationRaw, translationClean string
		if err := rows.Scan(&originalRaw, &originalClean, &translationRaw, &translationClean); err != nil {
			return nil, err
		}

		if originalClean == cleanWord {
			results = append(results, models.TranslationPairs{
				Original:  originalRaw,
				Translate: translationRaw,
			})
			continue
		}

		if translationClean == cleanWord {
			results = append(results, models.TranslationPairs{
				Original:  translationRaw,
				Translate: originalRaw,
			})
		}
	}

	return results, rows.Err()
}

// CountDictionaryPairs returns the total number of stored pairs and how many of
// them have been confirmed through moderation. Tracks dictionary growth — the
// core metric for the mission of expanding Chechen coverage.
func (r *Repository) CountDictionaryPairs(ctx context.Context) (total int, approved int, err error) {
	err = r.db.QueryRowContext(
		ctx,
		`SELECT
			COUNT(*),
			COUNT(CASE WHEN formatted_chosen IS NOT NULL AND formatted_chosen != 'deleted' THEN 1 END)
		 FROM dictionary_pairs;`,
	).Scan(&total, &approved)
	return total, approved, err
}

// GetPairCleanWords returns clean words (original + translation) for a pair by ID.
func (r *Repository) GetPairCleanWords(ctx context.Context, pairID int64) ([]string, error) {
	var origClean, transClean string
	err := r.db.QueryRowContext(ctx,
		`SELECT original_clean, translation_clean FROM dictionary_pairs WHERE id = ?;`,
		pairID,
	).Scan(&origClean, &transClean)
	if err != nil {
		return nil, err
	}
	return []string{origClean, transClean}, nil
}

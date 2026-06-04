package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
	"errors"
)

type TranslationPair struct {
	ID                  int64
	OriginalRaw         string
	OriginalClean       string
	OriginalLang        string
	TranslationRaw      string
	TranslationClean    string
	TranslationLang     string
	Source              string
	SourceEntryID       sql.NullString
	SourceTranslationID sql.NullString
	FormattedAI         sql.NullString
	FormattedChosen     sql.NullString
	FormatVersion       sql.NullString
}

const selectPairIDQuery = `select id from dictionary_pairs
	where original_clean = ? and original_lang = ?
	  and translation_clean = ? and translation_lang = ?
	limit 1;`

func (r *Repository) lookupPairID(ctx context.Context, pair TranslationPair) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(
		ctx,
		selectPairIDQuery,
		pair.OriginalClean,
		pair.OriginalLang,
		pair.TranslationClean,
		pair.TranslationLang,
	).Scan(&id)
	return id, err
}

// InsertTranslationPair stores a pair, reporting whether it was newly inserted
// or already existed so callers can skip re-processing duplicates. Duplicates
// are the common case — API fetches keep re-seeing stored pairs — so it checks
// read-only first instead of opening a write transaction per pair.
func (r *Repository) InsertTranslationPair(ctx context.Context, pair TranslationPair) (int64, bool, error) {
	existingID, err := r.lookupPairID(ctx, pair)
	if err == nil {
		return existingID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	result, err := r.db.ExecContext(
		ctx,
		`insert or ignore into dictionary_pairs (
			original_raw,
			original_clean,
			original_lang,
			translation_raw,
			translation_clean,
			translation_lang,
			source,
			source_entry_id,
			source_translation_id
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		pair.OriginalRaw,
		pair.OriginalClean,
		pair.OriginalLang,
		pair.TranslationRaw,
		pair.TranslationClean,
		pair.TranslationLang,
		pair.Source,
		pair.SourceEntryID,
		pair.SourceTranslationID,
	)
	if err != nil {
		return 0, false, err
	}

	// An ignored INSERT OR IGNORE must be detected via RowsAffected:
	// LastInsertId keeps the connection's previous rowid, so with a pooled
	// sql.DB it can return a stale ID belonging to an unrelated insert.
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected > 0 {
		id, err := result.LastInsertId()
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}

	// Lost an insert race with a concurrent writer; the row exists now.
	existingID, err = r.lookupPairID(ctx, pair)
	if err != nil {
		return 0, false, err
	}
	return existingID, false, nil
}

func (r *Repository) ListPendingTranslationPairs(ctx context.Context, limit int) ([]TranslationPair, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := r.db.QueryContext(
		ctx,
		`select
			id,
			original_raw,
			original_clean,
			original_lang,
			translation_raw,
			translation_clean,
			translation_lang,
			source,
			source_entry_id,
			source_translation_id,
			formatted_ai,
			formatted_chosen,
			format_version
		from dictionary_pairs
		where formatted_chosen is null and formatted_ai is not null
		limit ?;`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TranslationPair, 0, limit)
	for rows.Next() {
		var pair TranslationPair
		if err := rows.Scan(
			&pair.ID,
			&pair.OriginalRaw,
			&pair.OriginalClean,
			&pair.OriginalLang,
			&pair.TranslationRaw,
			&pair.TranslationClean,
			&pair.TranslationLang,
			&pair.Source,
			&pair.SourceEntryID,
			&pair.SourceTranslationID,
			&pair.FormattedAI,
			&pair.FormattedChosen,
			&pair.FormatVersion,
		); err != nil {
			return nil, err
		}
		result = append(result, pair)
	}

	return result, rows.Err()
}

func (r *Repository) ListPendingTranslationPairsByWord(ctx context.Context, cleanWord string, limit int) ([]TranslationPair, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := r.db.QueryContext(
		ctx,
		`select
			id,
			original_raw,
			original_clean,
			original_lang,
			translation_raw,
			translation_clean,
			translation_lang,
			source,
			source_entry_id,
			source_translation_id,
			formatted_ai,
			formatted_chosen,
			format_version
		from dictionary_pairs
		where formatted_chosen is null
		  and formatted_ai is not null
		  and (original_clean = ? or translation_clean = ?)
		limit ?;`,
		cleanWord, cleanWord, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]TranslationPair, 0, limit)
	for rows.Next() {
		var pair TranslationPair
		if err := rows.Scan(
			&pair.ID,
			&pair.OriginalRaw,
			&pair.OriginalClean,
			&pair.OriginalLang,
			&pair.TranslationRaw,
			&pair.TranslationClean,
			&pair.TranslationLang,
			&pair.Source,
			&pair.SourceEntryID,
			&pair.SourceTranslationID,
			&pair.FormattedAI,
			&pair.FormattedChosen,
			&pair.FormatVersion,
		); err != nil {
			return nil, err
		}
		result = append(result, pair)
	}

	return result, rows.Err()
}

func (r *Repository) SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error {
	_, err := r.db.ExecContext(
		ctx,
		`update dictionary_pairs
		set formatted_chosen = ?
		where id = ?;`,
		choice,
		id,
	)
	return err
}

func (r *Repository) UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error {
	var chosenVal any
	if formattedChosen != "" {
		chosenVal = formattedChosen
	}
	_, err := r.db.ExecContext(
		ctx,
		`update dictionary_pairs
		set formatted_ai = ?,
		    formatted_chosen = ?,
		    format_version = 'ai_v1'
		where id = ?;`,
		formattedAI,
		chosenVal,
		id,
	)
	return err
}

func (r *Repository) FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error) {
	if limit <= 0 {
		limit = 200
	}

	rows, err := r.db.QueryContext(
		ctx,
		`select
			original_raw,
			original_clean,
			translation_raw,
			translation_clean,
			formatted_ai,
			formatted_chosen
		from dictionary_pairs
		where formatted_chosen != 'deleted' and (original_clean = ? or translation_clean = ?)
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
		var formattedAI, formattedChosen sql.NullString
		if err := rows.Scan(&originalRaw, &originalClean, &translationRaw, &translationClean, &formattedAI, &formattedChosen); err != nil {
			return nil, err
		}

		var aiText, chosenText string
		if formattedAI.Valid {
			aiText = formattedAI.String
		}
		if formattedChosen.Valid {
			chosenText = formattedChosen.String
		}

		if originalClean == cleanWord {
			results = append(results, models.TranslationPairs{
				Original:        originalRaw,
				Translate:       translationRaw,
				FormattedAI:     aiText,
				FormattedChosen: chosenText,
			})
			continue
		}

		if translationClean == cleanWord {
			results = append(results, models.TranslationPairs{
				Original:        translationRaw,
				Translate:       originalRaw,
				FormattedAI:     aiText,
				FormattedChosen: chosenText,
			})
		}
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

func (r *Repository) StoreSpellcheckFeedback(ctx context.Context, userID int64, originalText, correctedText, feedback string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO spellcheck_feedback (user_id, original_text, corrected_text, feedback) VALUES (?, ?, ?, ?)`,
		userID, originalText, correctedText, feedback,
	)
	return err
}

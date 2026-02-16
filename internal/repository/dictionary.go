package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
	"strings"
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
	IsApproved          bool
	ModerationSentAt    sql.NullTime
	FormattedAI         sql.NullString
	FormattedChosen     sql.NullString
	FormatVersion       sql.NullString
}

func (r *Repository) InsertTranslationPair(ctx context.Context, pair TranslationPair) (int64, error) {
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
			source_translation_id,
			is_approved
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		pair.OriginalRaw,
		pair.OriginalClean,
		pair.OriginalLang,
		pair.TranslationRaw,
		pair.TranslationClean,
		pair.TranslationLang,
		pair.Source,
		pair.SourceEntryID,
		pair.SourceTranslationID,
		boolToInt(pair.IsApproved),
	)
	if err != nil {
		return 0, err
	}

	// Try to get last insert ID
	id, err := result.LastInsertId()
	if err != nil || id == 0 {
		// If insert was ignored (duplicate), fetch existing ID
		var existingID int64
		err = r.db.QueryRowContext(
			ctx,
			`select id from dictionary_pairs
			where original_clean = ? and original_lang = ?
			  and translation_clean = ? and translation_lang = ?
			limit 1;`,
			pair.OriginalClean,
			pair.OriginalLang,
			pair.TranslationClean,
			pair.TranslationLang,
		).Scan(&existingID)
		if err != nil {
			return 0, err
		}
		return existingID, nil
	}

	return id, nil
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
			is_approved,
			moderation_sent_at,
			formatted_ai,
			formatted_chosen,
			format_version
		from dictionary_pairs
		where is_approved = 0 and moderation_sent_at is null
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
		var isApproved int
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
			&isApproved,
			&pair.ModerationSentAt,
			&pair.FormattedAI,
			&pair.FormattedChosen,
			&pair.FormatVersion,
		); err != nil {
			return nil, err
		}
		pair.IsApproved = isApproved == 1
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
			is_approved,
			moderation_sent_at,
			formatted_ai,
			formatted_chosen,
			format_version
		from dictionary_pairs
		where is_approved = 0
		  and moderation_sent_at is null
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
		var isApproved int
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
			&isApproved,
			&pair.ModerationSentAt,
			&pair.FormattedAI,
			&pair.FormattedChosen,
			&pair.FormatVersion,
		); err != nil {
			return nil, err
		}
		pair.IsApproved = isApproved == 1
		result = append(result, pair)
	}

	return result, rows.Err()
}

func (r *Repository) MarkTranslationPairsSent(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	query, args := buildInClause("update dictionary_pairs set moderation_sent_at = current_timestamp where id in (", ids)
	query += ");"

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *Repository) SetTranslationPairApproval(ctx context.Context, id int64, approved bool, approvedBy string) error {
	// 0 = pending, 1 = approved, -1 = rejected
	approvedInt := 1
	if !approved {
		approvedInt = -1
	}
	_, err := r.db.ExecContext(
		ctx,
		`update dictionary_pairs
		set is_approved = ?,
		    approved_at = current_timestamp,
		    approved_by = ?
		where id = ?;`,
		approvedInt,
		approvedBy,
		id,
	)
	return err
}

func (r *Repository) SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string, approved bool, approvedBy string) error {
	approvedInt := 1
	if !approved {
		approvedInt = -1
	}
	_, err := r.db.ExecContext(
		ctx,
		`update dictionary_pairs
		set formatted_chosen = ?,
		    is_approved = ?,
		    approved_at = current_timestamp,
		    approved_by = ?
		where id = ?;`,
		choice,
		approvedInt,
		approvedBy,
		id,
	)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func buildInClause(prefix string, ids []int64) (string, []interface{}) {
	args := make([]interface{}, 0, len(ids))
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, "?")
	}
	return prefix + strings.Join(placeholders, ","), args
}

func (r *Repository) UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error {
	_, err := r.db.ExecContext(
		ctx,
		`update dictionary_pairs
		set formatted_ai = ?,
		    formatted_chosen = ?,
		    format_version = 'ai_v1'
		where id = ?;`,
		formattedAI,
		formattedChosen,
		id,
	)
	return err
}

func (r *Repository) FindApprovedTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error) {
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
		where is_approved >= 0 and (original_clean = ? or translation_clean = ?)
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

// FindStrictlyApprovedPairs returns only explicitly approved pairs (is_approved = 1)
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
		where is_approved = 1 and (original_clean = ? or translation_clean = ?)
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

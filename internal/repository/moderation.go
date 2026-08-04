// A stored pair's life after dosham answered with it: insertion, the review
// queue it lands in, and the moderator's verdict.
package repository

import (
	"chetoru/pkg/tools"
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
	// Rate is dosham's source-dictionary marker, and EntryType/Subtype/
	// EntryIndex/EntryNotes the entry structure the card renders from. They are
	// stored rather than kept only in the cache because the local table is the
	// steady-state read path: a word looked up twice must render identically.
	Rate       int
	EntryType  string
	Subtype    int
	EntryIndex int
	EntryNotes string
}

const selectPairIDQuery = `select id, rate from dictionary_pairs
	where original_clean = ? and original_lang = ?
	  and translation_clean = ? and translation_lang = ?
	limit 1;`

func (r *Repository) lookupPairID(ctx context.Context, pair TranslationPair) (int64, int, error) {
	var id int64
	var rate int
	err := r.db.QueryRowContext(
		ctx,
		selectPairIDQuery,
		pair.OriginalClean,
		pair.OriginalLang,
		pair.TranslationClean,
		pair.TranslationLang,
	).Scan(&id, &rate)
	return id, rate, err
}

// InsertTranslationPair stores a pair, reporting whether it was newly inserted
// or already existed so callers can skip re-processing duplicates. Duplicates
// are the common case — API fetches keep re-seeing stored pairs — so it checks
// read-only first instead of opening a write transaction per pair.
func (r *Repository) InsertTranslationPair(ctx context.Context, pair TranslationPair) (int64, bool, error) {
	existingID, existingRate, err := r.lookupPairID(ctx, pair)
	if err == nil {
		// Backfill once: rows stored before rate existed would otherwise keep
		// ordering at zero forever, since a stored pair is never re-inserted.
		// Guarded on rate = 0 so the common duplicate costs no write.
		if existingRate == 0 && pair.Rate > 0 {
			if _, err := r.db.ExecContext(
				ctx,
				`update dictionary_pairs set rate = ? where id = ? and rate = 0;`,
				pair.Rate, existingID,
			); err != nil {
				return existingID, false, err
			}
		}
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
			original_folded,
			original_lang,
			translation_raw,
			translation_clean,
			translation_folded,
			translation_lang,
			source,
			source_entry_id,
			source_translation_id,
			rate,
			entry_type,
			subtype,
			entry_index,
			entry_notes
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		pair.OriginalRaw,
		pair.OriginalClean,
		tools.FoldSearch(pair.OriginalClean),
		pair.OriginalLang,
		pair.TranslationRaw,
		pair.TranslationClean,
		tools.FoldSearch(pair.TranslationClean),
		pair.TranslationLang,
		pair.Source,
		pair.SourceEntryID,
		pair.SourceTranslationID,
		pair.Rate,
		pair.EntryType,
		pair.Subtype,
		pair.EntryIndex,
		pair.EntryNotes,
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
	existingID, _, err = r.lookupPairID(ctx, pair)
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

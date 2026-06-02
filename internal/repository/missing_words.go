package repository

import (
	"chetoru/internal/models"
	"context"
)

// RecordMissingWord registers a search that returned no translation.
// On repeat searches it increments the counter and refreshes the timestamp,
// so the most-wanted missing words bubble to the top.
func (r *Repository) RecordMissingWord(ctx context.Context, cleanWord, rawWord string) error {
	if cleanWord == "" {
		return nil
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO missing_words (clean_word, raw_word)
		 VALUES (?, ?)
		 ON CONFLICT(clean_word) DO UPDATE SET
		     search_count = search_count + 1,
		     last_searched_at = CURRENT_TIMESTAMP;`,
		cleanWord, rawWord,
	)
	return err
}

// CountMissingWords returns how many distinct words users searched for that had
// no translation — a measure of how much demand the dictionary still can't meet.
func (r *Repository) CountMissingWords(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM missing_words;`).Scan(&count)
	return count, err
}

// TopMissingWords returns the most-frequently searched words that have no
// translation yet, ordered by demand.
func (r *Repository) TopMissingWords(ctx context.Context, limit int) ([]models.MissingWord, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT clean_word, raw_word, search_count, last_searched_at
		 FROM missing_words
		 ORDER BY search_count DESC, last_searched_at DESC
		 LIMIT ?;`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []models.MissingWord
	for rows.Next() {
		var w models.MissingWord
		if err := rows.Scan(&w.CleanWord, &w.RawWord, &w.SearchCount, &w.LastSearchedAt); err != nil {
			return nil, err
		}
		words = append(words, w)
	}

	return words, rows.Err()
}

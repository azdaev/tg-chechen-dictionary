package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
)

// RandomApprovedPair returns a single random moderated dictionary pair, oriented
// so the Chechen side is the headword. Returns nil if the dictionary is empty.
// Powers the /random discovery feature — a low-friction way for users to learn
// new Chechen vocabulary instead of only looking up words they already know.
func (r *Repository) RandomApprovedPair(ctx context.Context) (*models.RandomWord, error) {
	var originalRaw, originalLang, translationRaw, translationLang string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT original_raw, original_lang, translation_raw, translation_lang
		 FROM dictionary_pairs
		 WHERE formatted_chosen IS NOT NULL AND formatted_chosen != 'deleted'
		 ORDER BY RANDOM()
		 LIMIT 1;`,
	).Scan(&originalRaw, &originalLang, &translationRaw, &translationLang)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	word := &models.RandomWord{}
	if originalLang == "CHE" {
		word.Chechen, word.Russian = originalRaw, translationRaw
	} else {
		word.Chechen, word.Russian = translationRaw, originalRaw
	}

	return word, nil
}

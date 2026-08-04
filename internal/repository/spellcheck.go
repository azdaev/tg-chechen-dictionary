// Spellcheck feedback, kept to measure whether the suggestions help.
package repository

import "context"

func (r *Repository) StoreSpellcheckFeedback(ctx context.Context, userID int64, originalText, correctedText, feedback string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO spellcheck_feedback (user_id, original_text, corrected_text, feedback) VALUES (?, ?, ?, ?)`,
		userID, originalText, correctedText, feedback,
	)
	return err
}

package repository

import (
	"context"
	"database/sql"
)

// RecordQuizAnswer increments a user's quiz tally, adding to correct_count only
// when the answer was right. Creates the row on first answer.
func (r *Repository) RecordQuizAnswer(ctx context.Context, userID int64, correct bool) error {
	inc := 0
	if correct {
		inc = 1
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO quiz_stats (user_id, correct_count, total_count)
		 VALUES (?, ?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET
		     correct_count = correct_count + ?,
		     total_count = total_count + 1,
		     updated_at = CURRENT_TIMESTAMP;`,
		userID, inc, inc,
	)
	return err
}

// GetQuizScore returns a user's lifetime correct/total quiz counts. Returns
// (0, 0, nil) for a user who has never answered.
func (r *Repository) GetQuizScore(ctx context.Context, userID int64) (correct int, total int, err error) {
	err = r.db.QueryRowContext(
		ctx,
		`SELECT correct_count, total_count FROM quiz_stats WHERE user_id = ?;`,
		userID,
	).Scan(&correct, &total)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return correct, total, err
}

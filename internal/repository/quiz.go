package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
)

// RecordQuizAnswer increments a user's quiz tally, adding to correct_count only
// when the answer was right. Creates the row on first answer and keeps the
// stored username fresh for the leaderboard.
func (r *Repository) RecordQuizAnswer(ctx context.Context, userID int64, username string, correct bool) error {
	inc := 0
	if correct {
		inc = 1
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO quiz_stats (user_id, username, correct_count, total_count)
		 VALUES (?, ?, ?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET
		     username = excluded.username,
		     correct_count = quiz_stats.correct_count + excluded.correct_count,
		     total_count = quiz_stats.total_count + 1,
		     updated_at = CURRENT_TIMESTAMP;`,
		userID, username, inc,
	)
	return err
}

// CountQuizStats returns aggregate quiz engagement: number of players and the
// total/correct answer counts across everyone.
func (r *Repository) CountQuizStats(ctx context.Context) (players, totalAnswers, correctAnswers int, err error) {
	err = r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_count), 0), COALESCE(SUM(correct_count), 0)
		 FROM quiz_stats;`,
	).Scan(&players, &totalAnswers, &correctAnswers)
	return players, totalAnswers, correctAnswers, err
}

// TopQuizScorers returns the leaderboard: players with the most correct answers.
// Only players with a meaningful number of attempts are included so the board
// reflects sustained practice rather than a single lucky guess.
func (r *Repository) TopQuizScorers(ctx context.Context, limit int) ([]models.QuizScorer, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT user_id, COALESCE(username, ''), correct_count, total_count
		 FROM quiz_stats
		 WHERE total_count >= 3
		 ORDER BY correct_count DESC, total_count ASC
		 LIMIT ?;`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scorers []models.QuizScorer
	for rows.Next() {
		var s models.QuizScorer
		if err := rows.Scan(&s.UserID, &s.Username, &s.Correct, &s.Total); err != nil {
			return nil, err
		}
		scorers = append(scorers, s)
	}

	return scorers, rows.Err()
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

package repository

import (
	"chetoru/internal/models"
	"context"
	"database/sql"
	"time"
)

// RecordQuizAnswer increments a user's quiz tally, adding to correct_count only
// when the answer was right. Creates the row on first answer, keeps the stored
// display names fresh for the leaderboard, and maintains the daily streak:
// same-day answers keep it, a yesterday record extends it, a gap resets it.
func (r *Repository) RecordQuizAnswer(ctx context.Context, userID int64, username, firstName string, correct bool) error {
	inc := 0
	if correct {
		inc = 1
	}
	now := time.Now()
	today := now.Format(time.DateOnly)
	yesterday := now.AddDate(0, 0, -1).Format(time.DateOnly)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO quiz_stats (user_id, username, first_name, correct_count, total_count, streak_days, last_answer_date)
		 VALUES (?, ?, ?, ?, 1, 1, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		     username = excluded.username,
		     first_name = excluded.first_name,
		     correct_count = quiz_stats.correct_count + excluded.correct_count,
		     total_count = quiz_stats.total_count + 1,
		     streak_days = CASE
		         WHEN quiz_stats.last_answer_date = excluded.last_answer_date THEN quiz_stats.streak_days
		         WHEN quiz_stats.last_answer_date = ? THEN quiz_stats.streak_days + 1
		         ELSE 1
		     END,
		     last_answer_date = excluded.last_answer_date,
		     updated_at = CURRENT_TIMESTAMP;`,
		userID, username, firstName, inc, today, yesterday,
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
		`SELECT user_id, COALESCE(username, ''), COALESCE(first_name, ''), correct_count, total_count
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
		if err := rows.Scan(&s.UserID, &s.Username, &s.FirstName, &s.Correct, &s.Total); err != nil {
			return nil, err
		}
		scorers = append(scorers, s)
	}

	return scorers, rows.Err()
}

// GetQuizScore returns a user's lifetime correct/total quiz counts and the
// current daily streak. Returns zeros for a user who has never answered. A
// streak whose last answer is older than yesterday has lapsed and reads as 0.
func (r *Repository) GetQuizScore(ctx context.Context, userID int64) (correct, total, streak int, err error) {
	var lastDate sql.NullString
	err = r.db.QueryRowContext(
		ctx,
		`SELECT correct_count, total_count, streak_days, last_answer_date FROM quiz_stats WHERE user_id = ?;`,
		userID,
	).Scan(&correct, &total, &streak, &lastDate)
	if err == sql.ErrNoRows {
		return 0, 0, 0, nil
	}
	now := time.Now()
	if lastDate.String != now.Format(time.DateOnly) && lastDate.String != now.AddDate(0, 0, -1).Format(time.DateOnly) {
		streak = 0
	}
	return correct, total, streak, err
}

package repository

import (
	entities "chetoru/internal/models"
	"context"
	"database/sql"
	"errors"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{
		db: db,
	}
}

const (
	insertUserQuery     = "INSERT OR IGNORE INTO users (user_id, username) VALUES (?, ?);"
	insertActivityQuery = "INSERT INTO activity (user_id, activity_type) VALUES (?, ?);"
	// The is_blocked guard keeps the unblock a no-op for the (vast) unblocked
	// majority: SQLite rewrites the row even when nothing changes otherwise.
	unblockUserQuery = "UPDATE users SET is_blocked = 0, blocked_at = null, blocked_reason = null WHERE user_id = ? AND is_blocked = 1;"
)

func (r *Repository) StoreUser(ctx context.Context, userID int, username string) error {
	_, err := r.db.ExecContext(ctx, insertUserQuery, userID, username)
	return err
}

// RecordUserActivity runs the per-interaction bookkeeping (user row, unblock
// flag, activity row) in one transaction: it fires on every message and inline
// keystroke, and three separate commits meant three WAL syncs where one does.
func (r *Repository) RecordUserActivity(ctx context.Context, userID int64, username string, activityType entities.ActivityType) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, insertUserQuery, userID, username); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, unblockUserQuery, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, insertActivityQuery, userID, activityType); err != nil {
		return err
	}
	return tx.Commit()
}

// WasInlineHinted reports whether the one-time inline-mode hint has been shown.
// An unknown user reads as hinted: the user row is written at the end of the
// same lookup, so the alternative is showing the hint again on the next one —
// which is the repetition this exists to stop.
func (r *Repository) WasInlineHinted(ctx context.Context, userID int64) (bool, error) {
	var v int
	err := r.db.QueryRowContext(ctx, "SELECT inline_hinted FROM users WHERE user_id = ?;", userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return v == 1, err
}

// MarkInlineHinted records that the inline-mode hint went out.
func (r *Repository) MarkInlineHinted(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE users SET inline_hinted = 1 WHERE user_id = ?;", userID)
	return err
}

// CountUserActivity returns how many lookups (text and inline) a user has made.
func (r *Repository) CountUserActivity(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM activity WHERE user_id = ?;", userID).Scan(&count)
	return count, err
}

func (r *Repository) ListUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT user_id FROM users WHERE is_blocked = 0;",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

func (r *Repository) MarkUserBlocked(ctx context.Context, userID int64, reason string) error {
	_, err := r.db.ExecContext(
		ctx,
		"UPDATE users SET is_blocked = 1, blocked_at = current_timestamp, blocked_reason = ? WHERE user_id = ?;",
		reason, userID,
	)
	return err
}

func (r *Repository) MarkUserUnblocked(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, unblockUserQuery, userID)
	return err
}

// monthRangeUTC returns the UTC half-open [start, end) bounds of a local
// calendar month, formatted like SQLite's current_timestamp. Range predicates
// on created_at can use its index, unlike the strftime('%m', ...) comparisons
// they replace, which scanned every row.
func monthRangeUTC(month, year int) (string, string) {
	const layout = "2006-01-02 15:04:05"
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	return start.UTC().Format(layout), start.AddDate(0, 1, 0).UTC().Format(layout)
}

func (r *Repository) CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error) {
	start, end := monthRangeUTC(month, year)
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(id) FROM users WHERE created_at >= ? AND created_at < ?;",
		start, end,
	)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *Repository) DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]entities.DailyActivity, error) {
	start, end := monthRangeUTC(month, year)
	result := make([]entities.DailyActivity, days)
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT day, COUNT(DISTINCT user_id) as \"dau\", COUNT(*) as \"calls\" FROM (SELECT user_id, strftime('%d', created_at, 'localtime') as \"day\" FROM activity WHERE created_at >= ? AND created_at < ?) GROUP BY day;",
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		day, dau, calls := 0, 0, 0
		err = rows.Scan(&day, &dau, &calls)
		if err != nil {
			return nil, err
		}

		if day < 1 || day > days {
			continue
		}

		result[day-1].ActiveUsers = dau
		result[day-1].Calls = calls
	}

	return result, nil
}

func (r *Repository) MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error) {
	start, end := monthRangeUTC(month, year)
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(DISTINCT user_id) FROM activity WHERE created_at >= ? AND created_at < ?;",
		start, end,
	)

	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

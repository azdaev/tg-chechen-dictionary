package repository

import (
	entities "chetoru/internal/models"
	"context"
	"database/sql"
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

func (r *Repository) StoreUser(ctx context.Context, userID int, username string) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT OR IGNORE INTO users (user_id, username) VALUES (?, ?);",
		userID, username,
	)
	return err
}

func (r *Repository) StoreActivity(ctx context.Context, userID int, activityType entities.ActivityType) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO activity (user_id, activity_type) VALUES (?, ?);",
		userID, activityType,
	)
	return err
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

// MarkUserUnblocked runs on every user interaction, so it must be a no-op for
// the (vast) unblocked majority: SQLite rewrites the row even when nothing
// changes, and the is_blocked guard keeps those commits from touching disk.
func (r *Repository) MarkUserUnblocked(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(
		ctx,
		"UPDATE users SET is_blocked = 0, blocked_at = null, blocked_reason = null WHERE user_id = ? AND is_blocked = 1;",
		userID,
	)
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

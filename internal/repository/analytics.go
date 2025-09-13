package repository

import (
	entities "chetoru/internal/models"
	"context"
	"database/sql"
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

func (r *Repository) CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error) {
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(id) FROM users WHERE strftime('%m', created_at) = ? AND strftime('%Y', created_at) = ?;",
		month, year,
	)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *Repository) DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]entities.DailyActivity, error) {
	result := make([]entities.DailyActivity, days)
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT day, COUNT(DISTINCT user_id) as \"dau\", COUNT(*) as \"calls\" FROM (SELECT user_id, strftime('%d', created_at) as \"day\"  FROM activity WHERE strftime('%m', created_at) = ? AND strftime('%Y', created_at) = ?) GROUP BY day;",
		month, year,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		day, dau, calls := 0, 0, 0
		err = rows.Scan(&day, &dau, &calls)
		if err != nil {
			return nil, err
		}

		result[day-1].ActiveUsers = dau
		result[day-1].Calls = calls
	}

	return result, nil
}

func (r *Repository) MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error) {
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(DISTINCT user_id) FROM activity WHERE strftime('%m', created_at) = ? AND strftime('%Y', created_at) = ?;",
		month, year,
	)

	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

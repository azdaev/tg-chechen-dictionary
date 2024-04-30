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
		"INSERT INTO users (user_id, username) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING;",
		userID, username,
	)
	return err
}

func (r *Repository) StoreActivity(ctx context.Context, userID int, activityType entities.ActivityType) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO activity (user_id, activity_type) VALUES ($1, $2);",
		userID, activityType,
	)
	return err
}

func (r *Repository) CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error) {
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(id) FROM users WHERE EXTRACT(MONTH FROM created_at) = $1 AND EXTRACT(YEAR FROM created_at) = $2;",
		month, year,
	)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *Repository) DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]int, error) {
	result := make([]int, days)
	rows, err := r.db.QueryContext(
		ctx,
		"SELECT COUNT(DISTINCT user_id), EXTRACT(DAY FROM created_at) FROM activity GROUP BY created_at HAVING EXTRACT(MONTH FROM created_at) = $1 AND EXTRACT(YEAR FROM created_at) = $2;",
		month, year,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		dau, day := 0, 0
		err = rows.Scan(&dau, &day)
		if err != nil {
			return nil, err
		}

		result[day-1] = dau
	}

	return result, nil
}

func (r *Repository) MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error) {
	count := 0
	row := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(DISTINCT user_id) FROM activity WHERE EXTRACT(MONTH FROM created_at) = $1 AND EXTRACT(YEAR FROM created_at) = $2;",
		month, year,
	)

	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

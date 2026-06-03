package repository

import (
	"context"
	"database/sql"
)

// SetWordOfDaySubscription opts a user in or out of the daily Word of the Day push.
func (r *Repository) SetWordOfDaySubscription(ctx context.Context, userID int64, subscribed bool) error {
	v := 0
	if subscribed {
		v = 1
	}
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users SET word_of_day_subscribed = ? WHERE user_id = ?;`,
		v, userID,
	)
	return err
}

// IsWordOfDaySubscribed reports whether a user is opted in. Unknown users are
// treated as not subscribed.
func (r *Repository) IsWordOfDaySubscribed(ctx context.Context, userID int64) (bool, error) {
	var v int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT word_of_day_subscribed FROM users WHERE user_id = ?;`,
		userID,
	).Scan(&v)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return v == 1, err
}

// CountWordOfDaySubscribers returns how many reachable (non-blocked) users are
// opted in to the daily push.
func (r *Repository) CountWordOfDaySubscribers(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM users WHERE word_of_day_subscribed = 1 AND is_blocked = 0;`,
	).Scan(&n)
	return n, err
}

// ListWordOfDaySubscribers returns the user IDs that should receive the daily
// push — opted in and not blocked.
func (r *Repository) ListWordOfDaySubscribers(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT user_id FROM users WHERE word_of_day_subscribed = 1 AND is_blocked = 0;`,
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

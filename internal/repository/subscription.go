package repository

import (
	"context"
	"time"
)

type Subscription struct {
	ID                int64
	UserID            int64
	Active            bool
	ExpiresAt         time.Time
	TelegramPaymentID string
	CreatedAt         time.Time
}

func (r *Repository) GetSpellcheckUsage(ctx context.Context, userID int64, month, year int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT count FROM spellcheck_usage WHERE user_id = ? AND month = ? AND year = ?`,
		userID, month, year,
	).Scan(&count)
	if err != nil {
		return 0, nil // no row = 0 usage
	}
	return count, nil
}

func (r *Repository) IncrementSpellcheckUsage(ctx context.Context, userID int64, month, year int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO spellcheck_usage (user_id, month, year, count) VALUES (?, ?, ?, 1)
		 ON CONFLICT(user_id, month, year) DO UPDATE SET count = count + 1`,
		userID, month, year,
	)
	return err
}

func (r *Repository) HasActiveSubscription(ctx context.Context, userID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM subscriptions WHERE user_id = ? AND active = 1 AND expires_at > datetime('now')`,
		userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) CreateSubscription(ctx context.Context, userID int64, expiresAt time.Time, telegramPaymentID string) error {
	// Deactivate old subscriptions
	_, _ = r.db.ExecContext(ctx,
		`UPDATE subscriptions SET active = 0 WHERE user_id = ?`,
		userID,
	)

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions (user_id, active, expires_at, telegram_payment_id) VALUES (?, 1, ?, ?)`,
		userID, expiresAt, telegramPaymentID,
	)
	return err
}

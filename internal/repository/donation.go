package repository

import (
	"context"
	"database/sql"
	"time"
)

func (r *Repository) StoreDonationMessage(ctx context.Context, userID int) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO donation_messages (user_id) VALUES ($1);",
		userID,
	)
	return err
}

func (r *Repository) GetLastDonationMessage(ctx context.Context, userID int) (time.Time, error) {
	var lastSent time.Time
	err := r.db.QueryRowContext(
		ctx,
		"SELECT sent_at FROM donation_messages WHERE user_id = $1 ORDER BY sent_at DESC LIMIT 1;",
		userID,
	).Scan(&lastSent)

	if err == sql.ErrNoRows {
		return time.Time{}, err
	}
	if err != nil {
		return time.Time{}, err
	}
	return lastSent, nil
}

func (r *Repository) ShouldSendDonationMessage(ctx context.Context, userID int) (bool, error) {
	lastSent, err := r.GetLastDonationMessage(ctx, userID)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	// If we found a last message, check if a week has passed
	return time.Since(lastSent) > 7*24*time.Hour, nil
}

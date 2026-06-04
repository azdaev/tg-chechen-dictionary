package repository

import (
	entities "chetoru/internal/models"
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newActivityTestRepo(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL UNIQUE,
		username TEXT,
		is_blocked INTEGER NOT NULL DEFAULT 0,
		blocked_at DATETIME,
		blocked_reason TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE activity (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		activity_type INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return NewRepository(db), db
}

// Timestamps are stored in UTC (SQLite current_timestamp) but months are
// counted in local time, so fixtures are built from local-month edges.
func TestMonthlyAnalytics_LocalMonthBounds(t *testing.T) {
	r, db := newActivityTestRepo(t)
	ctx := context.Background()

	utc := func(local time.Time) string {
		return local.UTC().Format("2006-01-02 15:04:05")
	}
	may1 := utc(time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local))
	may15 := utc(time.Date(2026, 5, 15, 12, 0, 0, 0, time.Local))
	april := utc(time.Date(2026, 4, 30, 23, 0, 0, 0, time.Local))
	june := utc(time.Date(2026, 6, 1, 0, 30, 0, 0, time.Local))

	addUser := func(id int64, createdAt string) {
		if _, err := db.Exec(`INSERT INTO users (user_id, created_at) VALUES (?, ?)`, id, createdAt); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	addActivity := func(userID int64, createdAt string) {
		if _, err := db.Exec(`INSERT INTO activity (user_id, activity_type, created_at) VALUES (?, 0, ?)`, userID, createdAt); err != nil {
			t.Fatalf("insert activity: %v", err)
		}
	}

	addUser(1, may15)
	addUser(2, april)
	addUser(3, june)

	addActivity(1, may15)
	addActivity(1, may15) // second call, same user and day
	addActivity(2, may1)
	addActivity(2, april)
	addActivity(3, june)

	newUsers, err := r.CountNewMonthlyUsers(ctx, 5, 2026)
	if err != nil || newUsers != 1 {
		t.Fatalf("CountNewMonthlyUsers = %d (err %v), want 1", newUsers, err)
	}

	mau, err := r.MonthlyActiveUsers(ctx, 5, 2026)
	if err != nil || mau != 2 {
		t.Fatalf("MonthlyActiveUsers = %d (err %v), want 2", mau, err)
	}

	daily, err := r.DailyActiveUsersInMonth(ctx, 5, 2026, 31)
	if err != nil {
		t.Fatalf("DailyActiveUsersInMonth: %v", err)
	}
	if d := daily[14]; d.ActiveUsers != 1 || d.Calls != 2 {
		t.Fatalf("day 15 = %d users / %d calls, want 1/2", d.ActiveUsers, d.Calls)
	}
	if d := daily[0]; d.ActiveUsers != 1 || d.Calls != 1 {
		t.Fatalf("day 1 = %d users / %d calls, want 1/1", d.ActiveUsers, d.Calls)
	}
	if d := daily[1]; d.ActiveUsers != 0 || d.Calls != 0 {
		t.Fatalf("day 2 = %d users / %d calls, want 0/0", d.ActiveUsers, d.Calls)
	}
}

func TestRecordUserActivity_BookkeepingInOneCall(t *testing.T) {
	r, db := newActivityTestRepo(t)
	ctx := context.Background()

	if err := r.RecordUserActivity(ctx, 7, "amadi", entities.ActivityTypeText); err != nil {
		t.Fatalf("RecordUserActivity: %v", err)
	}

	var users, activities int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE user_id = 7`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users = %d (err %v), want 1", users, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM activity WHERE user_id = 7`).Scan(&activities); err != nil || activities != 1 {
		t.Fatalf("activities = %d (err %v), want 1", activities, err)
	}

	// A blocked user interacting again must come back unblocked, and the
	// duplicate user insert must be ignored.
	if err := r.MarkUserBlocked(ctx, 7, "bot blocked"); err != nil {
		t.Fatalf("MarkUserBlocked: %v", err)
	}
	if err := r.RecordUserActivity(ctx, 7, "amadi", entities.ActivityTypeInline); err != nil {
		t.Fatalf("RecordUserActivity (repeat): %v", err)
	}

	var blocked int
	if err := db.QueryRow(`SELECT is_blocked FROM users WHERE user_id = 7`).Scan(&blocked); err != nil || blocked != 0 {
		t.Fatalf("is_blocked = %d (err %v), want 0", blocked, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE user_id = 7`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users after repeat = %d (err %v), want 1", users, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM activity WHERE user_id = 7`).Scan(&activities); err != nil || activities != 2 {
		t.Fatalf("activities after repeat = %d (err %v), want 2", activities, err)
	}
}

func TestMarkUserUnblocked_RestoresBlockedUser(t *testing.T) {
	r := newUsersTestRepo(t)
	ctx := context.Background()

	if err := r.StoreUser(ctx, 1, "alice"); err != nil {
		t.Fatalf("StoreUser: %v", err)
	}
	if err := r.MarkUserBlocked(ctx, 1, "bot blocked"); err != nil {
		t.Fatalf("MarkUserBlocked: %v", err)
	}
	ids, err := r.ListUserIDs(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("blocked user still listed: ids=%v err=%v", ids, err)
	}

	if err := r.MarkUserUnblocked(ctx, 1); err != nil {
		t.Fatalf("MarkUserUnblocked: %v", err)
	}
	ids, err = r.ListUserIDs(ctx)
	if err != nil || len(ids) != 1 {
		t.Fatalf("unblocked user missing: ids=%v err=%v", ids, err)
	}

	// Already-unblocked users are the common case; the call must stay a no-op.
	if err := r.MarkUserUnblocked(ctx, 1); err != nil {
		t.Fatalf("MarkUserUnblocked on unblocked user: %v", err)
	}
	ids, err = r.ListUserIDs(ctx)
	if err != nil || len(ids) != 1 {
		t.Fatalf("no-op unblock changed listing: ids=%v err=%v", ids, err)
	}
}

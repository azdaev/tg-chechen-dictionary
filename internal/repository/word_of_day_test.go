package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newUsersTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE users (
		user_id INTEGER PRIMARY KEY,
		username TEXT,
		is_blocked INTEGER NOT NULL DEFAULT 0,
		blocked_at DATETIME,
		blocked_reason TEXT,
		word_of_day_subscribed INTEGER NOT NULL DEFAULT 0,
		wotd_nudged INTEGER NOT NULL DEFAULT 0
	);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewRepository(db)
}

func TestWordOfDaySubscription(t *testing.T) {
	r := newUsersTestRepo(t)
	ctx := context.Background()

	if err := r.StoreUser(ctx, 1, "alice"); err != nil {
		t.Fatalf("StoreUser: %v", err)
	}

	// Default: not subscribed.
	if sub, err := r.IsWordOfDaySubscribed(ctx, 1); err != nil || sub {
		t.Fatalf("default subscribed = %v (err %v), want false", sub, err)
	}
	// Unknown user: not subscribed, no error.
	if sub, err := r.IsWordOfDaySubscribed(ctx, 999); err != nil || sub {
		t.Fatalf("unknown user subscribed = %v (err %v), want false", sub, err)
	}

	// Opt in.
	if err := r.SetWordOfDaySubscription(ctx, 1, true); err != nil {
		t.Fatalf("SetWordOfDaySubscription: %v", err)
	}
	if sub, _ := r.IsWordOfDaySubscribed(ctx, 1); !sub {
		t.Fatal("after opt-in, want subscribed")
	}

	// Opt out.
	if err := r.SetWordOfDaySubscription(ctx, 1, false); err != nil {
		t.Fatalf("SetWordOfDaySubscription: %v", err)
	}
	if sub, _ := r.IsWordOfDaySubscribed(ctx, 1); sub {
		t.Fatal("after opt-out, want not subscribed")
	}
}

func TestListWordOfDaySubscribers(t *testing.T) {
	r := newUsersTestRepo(t)
	ctx := context.Background()

	_ = r.StoreUser(ctx, 1, "alice")
	_ = r.StoreUser(ctx, 2, "bob")
	_ = r.StoreUser(ctx, 3, "carol")
	_ = r.SetWordOfDaySubscription(ctx, 1, true)
	_ = r.SetWordOfDaySubscription(ctx, 2, true)
	// carol stays unsubscribed; block bob even though subscribed.
	_ = r.MarkUserBlocked(ctx, 2, "test")

	subs, err := r.ListWordOfDaySubscribers(ctx)
	if err != nil {
		t.Fatalf("ListWordOfDaySubscribers: %v", err)
	}
	if len(subs) != 1 || subs[0] != 1 {
		t.Fatalf("subscribers = %v, want [1] (alice only; bob blocked, carol opted out)", subs)
	}

	// Count should match the reachable-subscriber list.
	n, err := r.CountWordOfDaySubscribers(ctx)
	if err != nil {
		t.Fatalf("CountWordOfDaySubscribers: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestWordOfDayNudge(t *testing.T) {
	r := newUsersTestRepo(t)
	ctx := context.Background()

	// Unknown users read as already nudged — never message them.
	nudged, err := r.WasWordOfDayNudged(ctx, 99)
	if err != nil || !nudged {
		t.Fatalf("unknown user nudged = %v (err %v), want true", nudged, err)
	}

	_ = r.StoreUser(ctx, 1, "alice")
	if nudged, _ = r.WasWordOfDayNudged(ctx, 1); nudged {
		t.Fatal("fresh user must not read as nudged")
	}

	if err := r.MarkWordOfDayNudged(ctx, 1); err != nil {
		t.Fatalf("MarkWordOfDayNudged: %v", err)
	}
	if nudged, _ = r.WasWordOfDayNudged(ctx, 1); !nudged {
		t.Fatal("marked user must read as nudged")
	}
}

func TestChatWordOfDaySubscription(t *testing.T) {
	r := newUsersTestRepo(t)
	ctx := context.Background()

	if _, err := r.db.Exec(`CREATE TABLE wotd_chats (
		chat_id INTEGER PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if sub, _ := r.IsChatWordOfDaySubscribed(ctx, -100); sub {
		t.Fatal("fresh chat must not be subscribed")
	}

	// Subscribe twice — second insert must be a no-op, not an error.
	for range 2 {
		if err := r.SetChatWordOfDaySubscription(ctx, -100, true); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}
	_ = r.SetChatWordOfDaySubscription(ctx, -200, true)

	if sub, _ := r.IsChatWordOfDaySubscribed(ctx, -100); !sub {
		t.Fatal("chat must be subscribed")
	}
	ids, err := r.ListWordOfDayChatIDs(ctx)
	if err != nil || len(ids) != 2 {
		t.Fatalf("chat ids = %v (err %v), want 2 chats", ids, err)
	}

	if err := r.SetChatWordOfDaySubscription(ctx, -100, false); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	ids, _ = r.ListWordOfDayChatIDs(ctx)
	if len(ids) != 1 || ids[0] != -200 {
		t.Fatalf("chat ids after unsubscribe = %v, want [-200]", ids)
	}
	if count, err := r.CountWordOfDayChats(ctx); err != nil || count != 1 {
		t.Fatalf("chat count = %d (err %v), want 1", count, err)
	}
}

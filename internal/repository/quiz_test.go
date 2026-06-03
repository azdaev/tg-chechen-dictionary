package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newQuizTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1) // keep the single in-memory connection alive
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE quiz_stats (
		user_id INTEGER PRIMARY KEY,
		correct_count INTEGER NOT NULL DEFAULT 0,
		total_count INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return NewRepository(db)
}

func TestQuizScore_NewUserIsZero(t *testing.T) {
	r := newQuizTestRepo(t)
	correct, total, err := r.GetQuizScore(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetQuizScore: %v", err)
	}
	if correct != 0 || total != 0 {
		t.Fatalf("new user score = %d/%d, want 0/0", correct, total)
	}
}

func TestQuizScore_AccumulatesAnswers(t *testing.T) {
	r := newQuizTestRepo(t)
	ctx := context.Background()

	for _, correct := range []bool{true, false, true, true, false} {
		if err := r.RecordQuizAnswer(ctx, 7, correct); err != nil {
			t.Fatalf("RecordQuizAnswer: %v", err)
		}
	}

	correct, total, err := r.GetQuizScore(ctx, 7)
	if err != nil {
		t.Fatalf("GetQuizScore: %v", err)
	}
	if correct != 3 || total != 5 {
		t.Fatalf("score = %d/%d, want 3/5", correct, total)
	}
}

func TestQuizScore_IsolatedPerUser(t *testing.T) {
	r := newQuizTestRepo(t)
	ctx := context.Background()

	_ = r.RecordQuizAnswer(ctx, 1, true)
	_ = r.RecordQuizAnswer(ctx, 2, false)

	c1, t1, _ := r.GetQuizScore(ctx, 1)
	c2, t2, _ := r.GetQuizScore(ctx, 2)

	if c1 != 1 || t1 != 1 {
		t.Fatalf("user 1 score = %d/%d, want 1/1", c1, t1)
	}
	if c2 != 0 || t2 != 1 {
		t.Fatalf("user 2 score = %d/%d, want 0/1", c2, t2)
	}
}

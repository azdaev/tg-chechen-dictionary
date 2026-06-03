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
		username TEXT,
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
		if err := r.RecordQuizAnswer(ctx, 7, "tester", correct); err != nil {
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

func TestTopQuizScorers(t *testing.T) {
	r := newQuizTestRepo(t)
	ctx := context.Background()

	record := func(userID int64, name string, correct, wrong int) {
		for range correct {
			_ = r.RecordQuizAnswer(ctx, userID, name, true)
		}
		for range wrong {
			_ = r.RecordQuizAnswer(ctx, userID, name, false)
		}
	}

	record(1, "alice", 5, 1) // 5/6 — leader
	record(2, "bob", 2, 3)   // 2/5
	record(3, "carol", 1, 1) // 1/2 — below the 3-attempt threshold, excluded

	board, err := r.TopQuizScorers(ctx, 10)
	if err != nil {
		t.Fatalf("TopQuizScorers: %v", err)
	}
	if len(board) != 2 {
		t.Fatalf("board size = %d, want 2 (carol excluded)", len(board))
	}
	if board[0].Username != "alice" || board[0].Correct != 5 {
		t.Fatalf("leader = %q %d, want alice 5", board[0].Username, board[0].Correct)
	}
	if board[1].Username != "bob" || board[1].Correct != 2 {
		t.Fatalf("runner-up = %q %d, want bob 2", board[1].Username, board[1].Correct)
	}
}

func TestCountQuizStats(t *testing.T) {
	r := newQuizTestRepo(t)
	ctx := context.Background()

	// Empty table.
	players, total, correct, err := r.CountQuizStats(ctx)
	if err != nil {
		t.Fatalf("CountQuizStats: %v", err)
	}
	if players != 0 || total != 0 || correct != 0 {
		t.Fatalf("empty stats = %d/%d/%d, want 0/0/0", players, total, correct)
	}

	// alice: 3 correct of 4; bob: 0 correct of 2.
	for _, c := range []bool{true, true, true, false} {
		_ = r.RecordQuizAnswer(ctx, 1, "alice", c)
	}
	for range 2 {
		_ = r.RecordQuizAnswer(ctx, 2, "bob", false)
	}

	players, total, correct, err = r.CountQuizStats(ctx)
	if err != nil {
		t.Fatalf("CountQuizStats: %v", err)
	}
	if players != 2 || total != 6 || correct != 3 {
		t.Fatalf("stats = %d players, %d total, %d correct; want 2/6/3", players, total, correct)
	}
}

func TestQuizScore_IsolatedPerUser(t *testing.T) {
	r := newQuizTestRepo(t)
	ctx := context.Background()

	_ = r.RecordQuizAnswer(ctx, 1, "alice", true)
	_ = r.RecordQuizAnswer(ctx, 2, "bob", false)

	c1, t1, _ := r.GetQuizScore(ctx, 1)
	c2, t2, _ := r.GetQuizScore(ctx, 2)

	if c1 != 1 || t1 != 1 {
		t.Fatalf("user 1 score = %d/%d, want 1/1", c1, t1)
	}
	if c2 != 0 || t2 != 1 {
		t.Fatalf("user 2 score = %d/%d, want 0/1", c2, t2)
	}
}

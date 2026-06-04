package repository

import (
	"context"
	"testing"
)

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

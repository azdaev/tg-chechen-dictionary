package repository

import (
	"context"
	"slices"
	"testing"
)

func TestWordForms(t *testing.T) {
	r := newDictionaryTestRepo(t)
	ctx := context.Background()

	// The paradigm as the grammar card receives it: long-vowel tildes the
	// keyboard cannot produce.
	if err := r.SaveWordForms(ctx, "лом", []string{"ло̃ьмаш", "ло̃ьман", "лом"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	// The user types the form flat; folding is what makes it meet.
	got, err := r.FindHeadwordsByForm(ctx, "лоьман", 3)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !slices.Contains(got, "лом") {
		t.Fatalf("got %q, want the lemma лом", got)
	}

	// The headword is not its own form — the folded columns already reach it,
	// and storing it here would open the same card twice.
	if got, err := r.FindHeadwordsByForm(ctx, "лом", 3); err != nil || len(got) != 0 {
		t.Fatalf("headword stored as its own form: %q %v", got, err)
	}

	// Re-saving is how every lookup behaves; it must not duplicate or fail.
	if err := r.SaveWordForms(ctx, "лом", []string{"ло̃ьман"}); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if got, err := r.FindHeadwordsByForm(ctx, "лоьман", 3); err != nil || len(got) != 1 {
		t.Fatalf("re-save duplicated the row: %q %v", got, err)
	}

	if got, err := r.FindHeadwordsByForm(ctx, "", 3); err != nil || got != nil {
		t.Fatalf("empty key must not scan the table: %q %v", got, err)
	}
}

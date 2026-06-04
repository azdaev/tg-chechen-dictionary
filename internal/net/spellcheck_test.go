package net

import "testing"

func TestInlineSpellDebounce_SupersededQuerySkipped(t *testing.T) {
	n := &Net{inlineSpellLatest: make(map[int64]string)}

	n.noteInlineSpellQuery(1, "q1")
	n.noteInlineSpellQuery(1, "q2") // user kept typing
	n.noteInlineSpellQuery(2, "other-user")

	if n.isLatestInlineSpellQuery(1, "q1") {
		t.Fatal("superseded query must be skipped")
	}
	if !n.isLatestInlineSpellQuery(1, "q2") {
		t.Fatal("latest query must run")
	}
	if n.isLatestInlineSpellQuery(1, "q2") {
		t.Fatal("settled query must not run twice")
	}
	if !n.isLatestInlineSpellQuery(2, "other-user") {
		t.Fatal("users must be debounced independently")
	}
}

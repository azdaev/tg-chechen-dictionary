package net

import (
	"strings"
	"testing"
)

func TestMoreCallbackData_FitsLimit(t *testing.T) {
	// Short word: button data is well within the 64-byte limit.
	if data, ok := moreCallbackData("дерево", 4); !ok || data != "more_дерево_4" {
		t.Errorf("moreCallbackData(дерево,4) = %q,%v; want more_дерево_4,true", data, ok)
	}

	// Long Cyrillic phrase (2 bytes/char) exceeds 64 bytes -> omitted.
	long := strings.Repeat("ӏ", 40) // 40 runes * 2 bytes = 80 bytes, plus prefix
	if _, ok := moreCallbackData(long, 4); ok {
		t.Errorf("moreCallbackData(long) ok = true, want false (would exceed 64-byte limit)")
	}

	// A produced payload that fits must round-trip through the parser.
	if data, ok := moreCallbackData("дерево", 8); ok {
		w, off, parsed := parseMoreCallback(data)
		if !parsed || w != "дерево" || off != 8 {
			t.Errorf("round-trip(%q) = %q,%d,%v; want дерево,8,true", data, w, off, parsed)
		}
	}
}

func TestParseMoreCallback(t *testing.T) {
	cases := []struct {
		data     string
		wantWord string
		wantOff  int
		wantOK   bool
	}{
		{"more_дерево_4", "дерево", 4, true},
		{"more_дерево_12", "дерево", 12, true},
		{"more_two_words_4", "two_words", 4, true}, // underscore in word preserved
		{"more_a_b_c_0", "a_b_c", 0, true},
		{"more__4", "", 0, false},        // empty word
		{"more_дерево_", "", 0, false},   // missing offset
		{"more_дерево_x", "", 0, false},  // non-numeric offset
		{"more_дерево_-1", "", 0, false}, // negative offset
		{"random_x_4", "", 0, false},     // wrong prefix
		{"", "", 0, false},
	}
	for _, c := range cases {
		w, off, ok := parseMoreCallback(c.data)
		if ok != c.wantOK || (ok && (w != c.wantWord || off != c.wantOff)) {
			t.Errorf("parseMoreCallback(%q) = %q,%d,%v; want %q,%d,%v",
				c.data, w, off, ok, c.wantWord, c.wantOff, c.wantOK)
		}
	}
}

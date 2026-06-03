package tools

import "testing"

func TestAlternateYo(t *testing.T) {
	cases := map[string]string{
		"елка": "ёлка", // е → ё
		"Елка": "Ёлка",
		"ёлка": "елка", // ё → е (users often type the canonical ё)
		"Ёжик": "Ежик",
		"дом":  "", // nothing to swap
		"":     "",
	}
	for in, want := range cases {
		if got := AlternateYo(in); got != want {
			t.Errorf("AlternateYo(%q) = %q, want %q", in, got, want)
		}
	}
}

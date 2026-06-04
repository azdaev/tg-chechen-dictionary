package tools

import (
	"slices"
	"testing"
)

func TestYoVariants(t *testing.T) {
	cases := map[string][]string{
		"елка":   {"ёлка"},             // single е → one variant
		"Елка":   {"Ёлка"},             // uppercase preserved
		"береза": {"бёреза", "берёза"}, // one variant per е position
		"ёлка":   {"елка"},             // ё → the all-е spelling
		"Ёжик":   {"Ежик"},
		"дом":    nil, // nothing to swap
		"":       nil,
	}
	for in, want := range cases {
		if got := YoVariants(in); !slices.Equal(got, want) {
			t.Errorf("YoVariants(%q) = %q, want %q", in, got, want)
		}
	}

	t.Run("capped", func(t *testing.T) {
		if got := YoVariants("еееееееее"); len(got) != maxYoVariants {
			t.Errorf("expected %d variants, got %d", maxYoVariants, len(got))
		}
	})
}

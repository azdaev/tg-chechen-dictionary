package tools

import (
	"slices"
	"strings"
	"testing"
)

func TestFoldSearch(t *testing.T) {
	cases := []struct{ in, want string }{
		// The whole point: a dropped palochka and the real spelling meet.
		{"Чӏегӏардиг", "чегардиг"},
		{"чегӏардиг", "чегардиг"},
		{"чегардиг", "чегардиг"},
		{"ч1ег1ардиг", "чегардиг"},
		// Combining long vowel (U+0303) and Russian stress (U+0301) go too — the
		// stored «лесто̃» used to be unreachable by typing «лесто».
		{"лесто̃", "лесто"},
		{"совеща́ние", "совещание"},
		// «й» must survive: NFD would split it and the Mn filter would eat the
		// breve, leaving «иоьшу».
		{"йоьшу", "йоьшу"},
	}
	for _, c := range cases {
		if got := FoldSearch(c.in); got != c.want {
			t.Errorf("FoldSearch(%q) = %q, want %q", c.in, got, c.want)
		}
		if again := FoldSearch(FoldSearch(c.in)); again != FoldSearch(c.in) {
			t.Errorf("FoldSearch is not idempotent on %q: %q", c.in, again)
		}
	}
}

func TestLooksChechen(t *testing.T) {
	for _, s := range []string{"чегӏардиг", "ч1ег1ардиг", "аьрзу", "хьоьжу", "буьйса"} {
		if !LooksChechen(s) {
			t.Errorf("LooksChechen(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"сущиствование", "яблоко", "берёза", ""} {
		if LooksChechen(s) {
			t.Errorf("LooksChechen(%q) = true — a Russian query must not pay for the cascade", s)
		}
	}
}

func TestPalochkaVariants(t *testing.T) {
	got := PalochkaVariants("чегӏардиг")
	if !slices.Contains(got, "чӏегӏардиг") {
		t.Fatalf("variants = %q, want the real spelling чӏегӏардиг among them", got)
	}
	if len(got) > maxPalochkaVariants {
		t.Errorf("variants = %d, over the cap of %d", len(got), maxPalochkaVariants)
	}
	seen := map[string]bool{}
	for _, v := range got {
		if seen[v] {
			t.Errorf("duplicate variant %q", v)
		}
		seen[v] = true
		if strings.Contains(v, "ӏӏ") {
			t.Errorf("variant %q doubled the palochka", v)
		}
	}
	if seen["чегӏардиг"] {
		t.Error("the query itself must not be retried")
	}

	// A consonant-initial query gets the word-initial glottal stop; a
	// vowel-initial one does not.
	if !slices.Contains(PalochkaVariants("баьрче"), "ӏбаьрче") {
		t.Error("word-initial insertion missing for a consonant-initial query")
	}
	for _, v := range PalochkaVariants("аьрзу") {
		if strings.HasPrefix(v, "ӏа") {
			t.Errorf("vowel-initial query got a word-initial palochka: %q", v)
		}
	}

	// Long carrier-heavy queries stay within the cap.
	if got := PalochkaVariants("цӏацӏацӏацӏацӏацӏа"); len(got) > maxPalochkaVariants {
		t.Errorf("cap not applied: %d variants", len(got))
	}
}

func TestRespellVariants(t *testing.T) {
	// A Chechen-looking query retries the palochka and never ё/е.
	for _, v := range RespellVariants("хьоьже", false) {
		if strings.ContainsRune(v, 'ё') {
			t.Errorf("Chechen query got a ё variant: %q", v)
		}
	}
	// A query the dictionary did answer retries ё/е only — no palochka spend on
	// a word that already has substring hits.
	yo := RespellVariants("береза", false)
	if len(yo) == 0 {
		t.Fatal("the ё/е branch stopped working")
	}
	for _, v := range yo {
		if strings.ContainsRune(v, 'ӏ') {
			t.Errorf("a query with results got a palochka variant: %q", v)
		}
	}

	// The case the gate cannot see: «гала» is «гӏала» with its only palochka
	// dropped, so nothing marks it as Chechen. A total miss has to recover it.
	if LooksChechen("гала") {
		t.Fatal("test premise broken: «гала» should not look Chechen")
	}
	if got := RespellVariants("гала", false); slices.Contains(got, "гӏала") {
		t.Errorf("a query with results should not spend palochka retries: %q", got)
	}
	if got := RespellVariants("гала", true); !slices.Contains(got, "гӏала") {
		t.Fatalf("unknown spelling did not reach the real word: %q", got)
	}
	if got := RespellVariants("гала", true); len(got) > maxPalochkaVariants {
		t.Errorf("combined cascade over the cap: %d variants", len(got))
	}
}

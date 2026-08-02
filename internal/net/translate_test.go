package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
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

func TestClampMessage(t *testing.T) {
	if got := clampMessage("короткий текст"); got != "короткий текст" {
		t.Errorf("short text must pass through, got %q", got)
	}

	// An oversized card is cut at a line boundary and marked with an ellipsis.
	line := strings.Repeat("ц", 80)
	long := strings.Repeat(line+"\n", 60)
	got := clampMessage(long)
	if n := len([]rune(got)); n > 3800+2 {
		t.Errorf("clamped length = %d runes, want <= 3802", n)
	}
	if !strings.HasSuffix(got, "\n…") {
		t.Errorf("clamped text must end with ellipsis, got %q", got[len(got)-20:])
	}
	for l := range strings.SplitSeq(strings.TrimSuffix(got, "\n…"), "\n") {
		if len([]rune(l)) != 80 {
			t.Errorf("clamp split a line: %d runes", len([]rune(l)))
		}
	}
}

// telegramMessageLimit is Telegram's own cap. Nothing in the bot may build a
// message past it: an oversized send is rejected outright, so the user gets
// nothing rather than a truncated answer.
const telegramMessageLimit = 4096

// HandleText appends the inline-mode hint AFTER clamping. The 3800-rune margin
// covers it, but that dependency is invisible at both sites — one edit to
// either and the longest cards start failing to send.
func TestClampedCardLeavesRoomForTheHelpText(t *testing.T) {
	clamped := clampMessage(strings.Repeat(strings.Repeat("ц", 80)+"\n", 60))
	total := len([]rune(clamped + "\n\n" + MoreTranslationsHelpText))
	if total >= telegramMessageLimit {
		t.Errorf("card plus help text = %d runes, want under %d", total, telegramMessageLimit)
	}
}

// The no-translation branch builds its own message from suggestions instead of
// going through the card path, so it needs its own clamp: three long glosses
// clear the limit and the near miss turns into a blank screen.
func TestNoTranslationWithSuggestionsIsClamped(t *testing.T) {
	suggestions := make([]models.TranslationPairs, 3)
	for i := range suggestions {
		suggestions[i] = models.TranslationPairs{
			Original:  "Яблоко",
			Translate: strings.Repeat("очень длинное толкование; ", 120),
		}
	}
	text := clampMessage(NoTranslationText + "\n\n" + SuggestionsHeaderText + "\n\n" + tools.FormatPairs(suggestions))
	if n := len([]rune(text)); n >= telegramMessageLimit {
		t.Errorf("suggestions message = %d runes, want under %d", n, telegramMessageLimit)
	}
	if !strings.HasPrefix(text, NoTranslationText) {
		t.Errorf("clamping ate the header: %q", text[:40])
	}
}

// The button carries the typed word, so a long one blows Telegram's 64-byte
// callback cap and the send fails with it — the whole miss message lost to a
// button nobody needed.
func TestCheckCallbackData_FitsLimit(t *testing.T) {
	data, ok := checkCallbackData("гӏала")
	if !ok || data != "check_гӏала" {
		t.Errorf("checkCallbackData(гӏала) = %q/%v", data, ok)
	}

	// isRecordableMissingWord allows up to 40 runes, and Cyrillic costs two
	// bytes each — so the cap is reachable through the front door.
	if _, ok := checkCallbackData(strings.Repeat("ц", 40)); ok {
		t.Error("a 40-rune word must be reported as too long to encode")
	}

	// Round-trips through the router's prefix, which is what the handler cuts.
	data, _ = checkCallbackData(" дитт ")
	if !strings.HasPrefix(data, "check_") {
		t.Errorf("data = %q, want the check_ prefix the router matches", data)
	}
	if word, _ := strings.CutPrefix(data, "check_"); word != "дитт" {
		t.Errorf("round-trip = %q, want the trimmed word", word)
	}
}

func TestIsRecordableMissingWord(t *testing.T) {
	cases := []struct {
		word string
		want bool
	}{
		{"дитт", true},
		{"ӏаж", true},                     // palochka counts as Cyrillic
		{"къинт1ера", true},               // "1" typed for palochka
		{"наьрташ дийцар", true},          // two-word term
		{"переведи мне это слово", false}, // sentence, not a term
		{"iphone", false},
		{"https://t.me/chetoru", false},
		{"дом2", false},
		{"123", false},
		{"а", false}, // single rune
		{"😀", false}, // no Cyrillic
		{"", false},
	}
	for _, c := range cases {
		if got := isRecordableMissingWord(c.word); got != c.want {
			t.Errorf("isRecordableMissingWord(%q) = %v, want %v", c.word, got, c.want)
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

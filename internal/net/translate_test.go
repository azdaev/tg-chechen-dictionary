package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"strings"
	"testing"
)

func TestParseMoreCallback_RoundTripsAPayload(t *testing.T) {
	// Nothing draws the button any more — the card is the whole answer — but the
	// handler stays for buttons sitting in older chats, so the parser must keep
	// reading what those chats send.
	w, off, ok := parseMoreCallback("more_дерево_8")
	if !ok || w != "дерево" || off != 8 {
		t.Errorf("parseMoreCallback = %q,%d,%v; want дерево,8,true", w, off, ok)
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

// Falling back to the line boundary only works when there is one. A single
// gloss past the limit is cut wherever the count runs out — and Telegram
// answers one broken tag by refusing the whole message, so the retry resends
// the card with no formatting at all.
func TestClampMessage_NeverCutsInsideMarkup(t *testing.T) {
	// The tag straddles the cut: 3799 runes of text, then "<b>…".
	inTag := clampMessage(strings.Repeat("ц", 3799) + "<b>слово</b>")
	if strings.LastIndex(inTag, "<") > strings.LastIndex(inTag, ">") {
		t.Errorf("cut left a half-written tag: %q", inTag[len(inTag)-20:])
	}

	// And an opened tag that the cut orphaned gets closed.
	orphan := clampMessage("<b>" + strings.Repeat("ц", 4000) + "</b>")
	if strings.Count(orphan, "<b>") != strings.Count(orphan, "</b>") {
		t.Errorf("clamped card leaves <b> open: %q", orphan[:20]+"…"+orphan[len(orphan)-20:])
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

// The miss message goes out with ParseMode html, so an unbalanced tag in the
// palochka hint would not garble one line — Telegram rejects the whole message
// and the user sees nothing at all.
func TestPalochkaHintIsValidHTML(t *testing.T) {
	if strings.Count(PalochkaHintText, "<") != strings.Count(PalochkaHintText, ">") {
		t.Fatalf("unbalanced brackets: %q", PalochkaHintText)
	}
	if got := tools.EscapeUnclosedTags(PalochkaHintText); got != PalochkaHintText {
		t.Fatalf("hint would be mangled before sending: %q", got)
	}
	// The hint only rides along on Chechen-looking misses; a Russian typo must
	// not be told about a letter it has no use for.
	if !tools.LooksChechen("чегӏардиг") || tools.LooksChechen("сущиствование") {
		t.Fatal("the gate that decides whether the hint is shown is broken")
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

// The palochka hint teaches how to type «ӏ» without the key. Gated on the query
// merely looking Chechen, it went to people who had just typed two of them —
// on a miss that already carries the apology, the note about the report, the
// neighbours and the suggestions.
func TestPalochkaHint_OnlyForSomeoneWhoDidNotTypeOne(t *testing.T) {
	cases := []struct {
		word string
		want bool
	}{
		{"куьг", true},           // Chechen by its vowels, no palochka typed
		{"чӏегӏардиг", false},    // typed two of them already
		{"доьхьал", true},        // Chechen by its vowels; «хь» is not a palochka
		{"гӏала", false},         // has one
		{"г1ала", false},         // typed as a digit; the key normalizes it to «ӏ»
		{"сущиствование", false}, // Russian: never got the hint, still does not
	}
	for _, c := range cases {
		clean := tools.NormalizeSearch(c.word)
		got := tools.LooksChechen(clean) && !strings.ContainsRune(clean, 'ӏ')
		if got != c.want {
			t.Errorf("hint for %q = %v, want %v", c.word, got, c.want)
		}
	}
}

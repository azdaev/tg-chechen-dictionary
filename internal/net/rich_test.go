package net

import (
	"chetoru/internal/models"
	"strings"
	"testing"
)

// The plain assembly has to stay exactly what it was: a rich send can fail, and
// what the user gets then is this.
func TestCardBuilder_PlainIsBlankLines(t *testing.T) {
	b := cardBuilder{}
	b.quote("<i>по запросу «ваха»:</i>")
	b.body("<b>даха</b>\n1. жить")
	b.line("<i>рядом:</i> дахар")
	want := "<i>по запросу «ваха»:</i>\n\n<b>даха</b>\n1. жить\n\n<i>рядом:</i> дахар"
	if got := b.String(); got != want {
		t.Errorf("plain card =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(b.String(), "<footer>") {
		t.Error("the plain card grew a footer it cannot render")
	}
}

// The rich assembly separates with tags instead, and closes with the licence
// footer dosham asks for.
func TestCardBuilder_RichWrapsEachPiece(t *testing.T) {
	b := cardBuilder{rich: true}
	b.quote("по запросу «ваха»")
	b.body("<h3>даха</h3>")
	b.line("рядом: дахар")
	b.credit()
	got := b.String()
	want := "<blockquote>по запросу «ваха»</blockquote><h3>даха</h3><p>рядом: дахар</p><footer>Словарь dosham.app</footer>"
	if got != want {
		t.Errorf("rich card =\n%q\nwant\n%q", got, want)
	}

	// The attribution is opt-in: crediting a dictionary under «нет перевода»
	// would name a source for saying nothing.
	uncredited := cardBuilder{rich: true}
	uncredited.line(NoTranslationText)
	if strings.Contains(uncredited.String(), "<footer>") {
		t.Errorf("a card that shows no dictionary content still credited one: %s", uncredited.String())
	}
	// An empty piece must not leave an empty tag behind.
	empty := cardBuilder{rich: true}
	empty.body("<h3>x</h3>")
	empty.line("")
	if strings.Contains(empty.String(), "<p></p>") {
		t.Errorf("an absent piece emitted an empty paragraph: %s", empty.String())
	}
}

// The miss card was hand-glued with «text += "\n\n" + …» before it moved onto
// cardBuilder. The rich dialect is new; the plain one must be the same string
// it always was, because that is what the overwhelming majority of users get.
func TestMissCard_PlainAssemblyUnchanged(t *testing.T) {
	b := cardBuilder{}
	b.line(NoTranslationText)
	b.line(MissingWordRecordedText)
	b.quote(PalochkaHintText)
	b.line("<i>рядом:</i> гӏала")
	b.line(SuggestionsHeaderText)
	b.line("гӏала — башня")

	// Exactly the old concatenation, spelled out.
	want := NoTranslationText +
		"\n\n" + MissingWordRecordedText +
		"\n\n" + PalochkaHintText +
		"\n\n" + "<i>рядом:</i> гӏала" +
		"\n\n" + SuggestionsHeaderText +
		"\n\n" + "гӏала — башня"
	if got := b.String(); got != want {
		t.Errorf("plain miss card changed:\n got: %q\nwant: %q", got, want)
	}
}

// headwordLine decides whether the grammar block has to name its own word. A
// rich card is one long string with no newlines, so the line-splitting version
// returned the whole card — and «is the headword somewhere in this string» is
// true of almost any card, which silently dropped the header from every rich
// card that answered under a different word.
func TestHeadwordLine_ReadsTheRichHeading(t *testing.T) {
	rich := "<blockquote>по запросу «ваха»</blockquote><h3>даха — <i>чеченский</i></h3>" +
		"<p>жить</p><table compact><tr><td>ваха хӏусам</td><td>жилой дом</td></tr></table>"
	if got := headwordLine(rich); got != "даха — <i>чеченский</i>" {
		t.Errorf("headwordLine(rich) = %q, want the <h3> contents", got)
	}
	// The plain card still works the old way: the heading is the line with the
	// direction chip, not the «по запросу» line above it.
	plain := "<i>по запросу «ваха»:</i>\n\n<b>даха</b> — <i>чеченский</i>\nжить"
	if got := headwordLine(plain); !strings.Contains(got, "даха") {
		t.Errorf("headwordLine(plain) = %q, want the headword line", got)
	}
}

func TestFirstTagText(t *testing.T) {
	if got, ok := firstTagText("<h3>а</h3><h3>б</h3>", "h3"); !ok || got != "а" {
		t.Errorf("firstTagText = %q, %v; want the first heading", got, ok)
	}
	if _, ok := firstTagText("<h3>оборван", "h3"); ok {
		t.Error("an unclosed tag reported a match")
	}
	if _, ok := firstTagText("нет тегов", "h3"); ok {
		t.Error("a card with no heading reported one")
	}
}

// The grammar block folds into a disclosure widget, and says nothing when it
// has nothing the card does not already carry.
func TestFormatGrammarRich(t *testing.T) {
	g := &models.WordGrammar{
		Headword: "жӏаьла", POS: "существительное",
		Forms:  []string{"жӏаьлеш", "жӏаьлин"},
		Idioms: []models.Idiom{{Chechen: "жӏаьлин гуй", Russian: "кормушка для собак"}},
	}
	got := formatGrammarRich(g, "<h3>собака</h3><p><b>жӏаьла</b></p>")
	for _, want := range []string{
		"<details><summary>Формы и выражения</summary>",
		"Формы: жӏаьлеш, жӏаьлин",
		"<td>жӏаьлин гуй</td><td>кормушка для собак</td>",
		"<th>чеченский</th>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("grammar block is missing %q:\n%s", want, got)
		}
	}

	// Nothing but a header repeating a word the card already leads with.
	bare := &models.WordGrammar{Headword: "жӏаьла"}
	if block := formatGrammarRich(bare, "<h3>жӏаьла</h3>"); block != "" {
		t.Errorf("a block with nothing to add was still sent: %q", block)
	}

	// An expression the card already lists is not repeated.
	card := "<h3>собака</h3><table compact><tr><td>жӏаьлин гуй</td><td>кормушка</td></tr></table>"
	if block := formatGrammarRich(g, card); strings.Contains(block, "жӏаьлин гуй") {
		t.Errorf("an expression already on the card was repeated:\n%s", block)
	}
}

// richUsable is the last gate before a card goes out as rich: an oversized one
// falls back rather than being cut, because clampMessage knows how to close <b>
// and nothing about a truncated <table>.
func TestRichUsable(t *testing.T) {
	defer func(v bool) { richMessages = v }(richMessages)

	richMessages = false
	if richUsable("<h3>x</h3>") {
		t.Error("rich was used with the feature switched off")
	}
	richMessages = true
	if !richUsable("<h3>x</h3>") {
		t.Error("a normal card was rejected")
	}
	if richUsable("") {
		t.Error("an empty card was accepted")
	}
	if richUsable(strings.Repeat("a", richLimit+1)) {
		t.Error("a card over Telegram's rich limit was accepted")
	}
}

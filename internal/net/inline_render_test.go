package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestInlineCardRendering(t *testing.T) {
	// The inline-sent message must match the text path's card, not the raw
	// gloss; the picker description is built from the data behind it.
	p := models.TranslationPairs{
		Original:      "Дом",
		Translate:     "м 1) цӏа; деревянный ~- дечиган цӏа",
		OriginalLang:  "RUS",
		TranslateLang: "CHE",
		EntryType:     "WORD",
		Rate:          100,
	}
	formatted := tools.FormatCard(p.Original, []models.TranslationPairs{p})
	if !strings.HasPrefix(formatted, "дом · <i>рус. → чеч.</i>\n<b>цӏа</b>") {
		t.Errorf("formatted card = %q, want it to open with the headword and its gloss", formatted)
	}
	if strings.Contains(formatted, "~") || strings.Contains(formatted, "1)") {
		t.Errorf("raw gloss markup leaked into inline card:\n%s", formatted)
	}

	// Telegram renders the description as plain text, so a tag reaching it
	// shows up literally — which is what slicing the description out of the
	// rendered card started doing the moment that card grew a <b>.
	desc := inlineDescription(p.Translate)
	if strings.ContainsAny(desc, "<>") {
		t.Errorf("description carries markup: %q", desc)
	}
	if strings.Contains(desc, "\n") {
		t.Errorf("description not condensed to one line: %q", desc)
	}
	if !strings.Contains(desc, "цӏа") {
		t.Errorf("description lost the translation: %q", desc)
	}
}

func TestInlineDescriptionTruncates(t *testing.T) {
	desc := inlineDescription(strings.Repeat("слово ", 40))
	if n := len([]rune(desc)); n > inlineDescriptionRunes+1 { // +1 for the ellipsis
		t.Errorf("description is %d runes, want at most %d", n, inlineDescriptionRunes+1)
	}
	if !strings.HasSuffix(desc, "…") {
		t.Errorf("truncated description missing its ellipsis: %q", desc)
	}
	// The cut lands on a word boundary, so no half-word before the ellipsis.
	if strings.HasSuffix(desc, "сло…") {
		t.Errorf("description cut mid-word: %q", desc)
	}
}

// An outage used to answer nothing at all, leaving a dead picker with no
// explanation. Saying so is only safe if Telegram's edge does not keep the
// message after the outage ends — which is what CacheTime 0 and IsPersonal buy.
func TestInlineUnavailableIsNotEdgeCached(t *testing.T) {
	conf := inlineUnavailableConfig("q1")
	if conf.CacheTime != 0 {
		t.Errorf("CacheTime = %d, want 0 — an outage must not outlive itself", conf.CacheTime)
	}
	if !conf.IsPersonal {
		t.Error("IsPersonal = false; a shared cache entry would show the outage to everyone")
	}
	if len(conf.Results) != 1 {
		t.Fatalf("got %d results, want one explaining the failure", len(conf.Results))
	}
}

// Whether a lookup succeeded is one question with one answer. The picker used
// to ask a different one — "did dosham return any rows?" — while the chat asks
// the card whether anything actually matched. So a query dosham answers with
// nothing but noise was «нет перевода» in a chat and a list of results in the
// picker, from the same data.
func TestInlineAgreesWithTheChatAboutAMiss(t *testing.T) {
	// «стрим» inside «гольфстрим»: dosham returns the row, the card refuses it.
	noise := []models.TranslationPairs{
		{Original: "Гольфстрим", Translate: "м Гольфстрим", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100},
	}
	if card, _ := tools.Card("стрим", noise); card != "" {
		t.Fatalf("the chat path treats this as an answer, so the test proves nothing: %q", card)
	}
	// The rule the picker used to apply. Kept as an assertion so the divergence
	// this test pins stays visible: on this input the two rules disagree, which
	// is what made one interface answer «нет перевода» and the other a list.
	if len(noise) == 0 {
		t.Fatal("the old rule would call this a miss too, so the test proves nothing")
	}
	if got := inlineArticles("q", "стрим", noise, false); len(got) != 0 {
		t.Errorf("picker offered %d results for what the chat calls a miss", len(got))
	}

	// And the agreement holds the other way: a real hit is offered.
	hit := []models.TranslationPairs{
		{Original: "Дом", Translate: "м 1) цӏа; деревянный ~- дечиган цӏа", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100},
	}
	got := inlineArticles("q", "дом", hit, false)
	if len(got) < 2 {
		t.Fatalf("got %d rows, want the whole card plus one per entry", len(got))
	}

	// The first row is the whole card — the one carrying the direction chip and
	// the examples. Picking any row must send a card, never a raw gloss.
	first, ok := got[0].(tgbotapi.InlineQueryResultArticle)
	if !ok {
		t.Fatalf("first result is %T", got[0])
	}
	text := first.InputMessageContent.(tgbotapi.InputTextMessageContent).Text
	if !strings.Contains(text, "рус. → чеч.") || !strings.Contains(text, "дечиган цӏа") {
		t.Errorf("the lead row is not the whole card:\n%s", text)
	}
	if strings.ContainsAny(first.Description, "<>") {
		t.Errorf("markup leaked into the picker subtitle: %q", first.Description)
	}
}

// Suggestions are other words, so they have no card for the query and must not
// be silenced by the miss test above.
func TestInlineSuggestionsSurvive(t *testing.T) {
	got := inlineArticles("q", "яблоками", []models.TranslationPairs{
		{Original: "Яблоко", Translate: "с Ӏаж", OriginalLang: "RUS", TranslateLang: "CHE", EntryType: "WORD", Rate: 100},
	}, true)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want the one suggestion", len(got))
	}
	if title := got[0].(tgbotapi.InlineQueryResultArticle).Title; !strings.HasPrefix(title, "🔍 ") {
		t.Errorf("suggestion not marked as a guess: %q", title)
	}
}

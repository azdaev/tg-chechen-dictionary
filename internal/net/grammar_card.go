// The grammar block grown into a card: declension forms and set expressions.
package net

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// maxGrammarForms caps how many inflected forms the grammar card lists, so a
// word with a long paradigm doesn't produce an overwhelming message.
const maxGrammarForms = 12

// sendGrammarCard looks up grammar for the Chechen headword behind a query and,
// if any is available, sends a compact follow-up card as a reply to the
// translation it belongs to — unattached, it reads as a message about nothing.
// It is a no-op when the word has no analyzed grammar, so most TEXT/phrase
// lookups send nothing.
//
// base, when set, is the word this entry is derived from: looking up what it
// means is a second dictionary round trip, and this is where the enrichments
// that must not sit between the user and their translation are paid for.
func (n *Net) sendGrammarCard(ctx context.Context, chatID int64, messageID int, card, word, base string) {
	var extra []string
	if base != "" {
		if line := n.baseWordLine(base); line != "" {
			extra = append(extra, line)
		}
	}
	// Optional enrichment on top of a translation already delivered, so a
	// failure just means no card — logged, never surfaced.
	if g, err := n.business.GrammarFor(ctx, word); err != nil {
		n.log.WithError(err).WithField("word", word).Debug("grammar lookup failed")
	} else if block := formatGrammarBlock(g, card); block != "" {
		extra = append(extra, block)
	}
	if len(extra) == 0 {
		return
	}
	// Grown into the translation rather than sent after it. As a second message
	// it arrived whenever the API answered, which in a fast exchange put the
	// grammar for one word underneath the answer to the next one.
	edit := tgbotapi.NewEditMessageText(chatID, messageID, clampMessage(card+"\n\n"+strings.Join(extra, "\n\n")))
	edit.ParseMode = "html"
	if _, err := n.send(edit); err != nil {
		n.log.WithError(err).Debug("failed to append grammar to the card")
	}
}

// formatGrammarCard renders a WordGrammar as a small Telegram-HTML card. Only
// facts safe to show without the dosham integer-code legend are included: the
// part of speech (when confidently known) and the inflected forms.
func formatGrammarBlock(g *models.WordGrammar, card string) string {
	if g == nil || g.Headword == "" {
		return ""
	}

	var lines []string
	// The header only earns its place when the card above is not already headed
	// by this word: appended to «телефон · сущ.», a «🔤 телефон · существительное»
	// line says the same thing twice. Anywhere else in the card is not enough —
	// for a Russian query the paradigm belongs to one of the glosses, and an
	// unlabelled «Формы:» under «карандаш» reads as the forms of «карандаш».
	// 🔤, not 📖: the book belongs to the Word of the Day, and a subscriber who
	// gets both opens the grammar card reading it as today's word.
	headerNeeded := !strings.Contains(headwordLine(card), tools.Clean(g.Headword))
	if headerNeeded {
		header := "🔤 <b>" + tools.Clean(g.Headword) + "</b>"
		if g.POS != "" {
			header += " · " + g.POS
		}
		lines = append(lines, header)
	}

	if len(g.Forms) > 0 {
		forms := g.Forms
		more := 0
		if len(forms) > maxGrammarForms {
			more = len(forms) - maxGrammarForms
			forms = forms[:maxGrammarForms]
		}
		cleaned := make([]string, 0, len(forms))
		for _, f := range forms {
			cleaned = append(cleaned, tools.Clean(f))
		}
		line := "Формы: " + strings.Join(cleaned, ", ")
		if more > 0 {
			line += fmt.Sprintf(" … (+%d)", more)
		}
		lines = append(lines, line)
	}

	// Set phrases the card already lists are dropped: «телефон» used to answer
	// with three examples and then repeat all three under «Выражения».
	//
	// Compared folded, because the corpora disagree about the marks: «рука»
	// answered with «беран куьг лаца» and then repeated it as «бе̃ран куьг ла̃ца»,
	// the same phrase spelled by the academic corpus, which reads as two.
	folded := tools.FoldSearch(card)
	var idioms []string
	for _, idiom := range g.Idioms {
		che := tools.Clean(idiom.Chechen)
		if strings.Contains(folded, tools.FoldSearch(che)) {
			continue
		}
		idioms = append(idioms, "• "+tools.FormatExample(che, tools.Clean(idiom.Russian)))
	}
	if len(idioms) > 0 {
		lines = append(lines, "\n💬 <b>Выражения:</b>")
		lines = append(lines, idioms...)
	}

	if len(lines) == 0 || (headerNeeded && len(lines) == 1 && g.POS == "") {
		return "" // nothing the card does not already say
	}
	return strings.Join(lines, "\n")
}

// headwordLine finds the line a card is headed by. Not simply the first line:
// a card that answered under a different word than the one typed opens with
// «по запросу «ваха»:», and reading that as the heading made every redirected
// card repeat its own headword in the grammar block below it. Every block
// header carries the direction chip, and nothing else on a card does.
func headwordLine(card string) string {
	for _, line := range strings.Split(card, "\n") {
		if strings.Contains(line, " · ") {
			return line
		}
	}
	head, _, _ := strings.Cut(card, "\n")
	return head
}

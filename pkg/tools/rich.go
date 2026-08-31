package tools

import (
	"chetoru/internal/models"
	"fmt"
	"strings"
)

// The same card in Bot API rich-message HTML. Structure the plain card can only
// imply — this line is a heading, these are the senses, this column is Chechen —
// is stated in tags the client renders itself:
//
//	<h3>заголовок</h3><p><i>пометы</i></p>
//	<ol><li>смысл</li></ol>
//	<table compact><tr><th>чеченский</th><th>русский</th></tr>…</table>
//	<details><summary>Ещё N примеров</summary>…</details>
//
// Only the markup differs. Wording, ordering and every judgement about what
// belongs on a card are the plain renderer's, unchanged, so the two cards always
// say the same thing.
const (
	// Rows shown before the rest fold into <details>. The plain card stops at
	// maxCardExampleLines and drops the remainder; here it is one tap away.
	maxRichExampleRows = 6
	// Total rows kept. Rich messages allow 500 blocks and a table row is a
	// block, so this is about the reader, not the API.
	maxRichExamples = 24
)

// RenderRich is Render with a rich-message body. A separate entry point rather
// than a second field on Rendered: the inline picker cannot send rich messages
// at all, and it runs on every keystroke, so it must not pay for one.
func RenderRich(query string, pairs []models.TranslationPairs) Rendered {
	return renderWith(query, pairs, collected.renderRich)
}

func (c collected) renderRich() string {
	out := make([]string, 0, len(c.blocks))
	for _, b := range c.blocks {
		out = append(out, b.renderRich())
	}
	return strings.Join(out, "")
}

func (b *block) renderRich() string {
	var sb strings.Builder

	name, quals := b.display()
	sb.WriteString("<h3>" + name + b.homonym())
	if len(quals) > 0 {
		sb.WriteString(" <i>(" + strings.Join(quals, ", ") + ")</i>")
	}
	sb.WriteString(tag(b.headLang()) + "</h3>")

	// Bold still marks the Chechen side and only that. Under a Chechen headword
	// the senses are Russian and carry none — the <h3> is what sets the headword
	// apart there, so bold is free to keep its one meaning.
	senses := b.senseLines()
	switch {
	case len(senses) == 1:
		sb.WriteString("<p>" + senses[0] + "</p>")
	case len(senses) > 1:
		sb.WriteString("<ol>")
		for _, s := range senses {
			sb.WriteString("<li>" + s + "</li>")
		}
		sb.WriteString("</ol>")
	}

	examples := b.orderedExamples()
	if len(examples) > maxRichExamples {
		examples = examples[:maxRichExamples]
	}
	if len(examples) == 0 {
		return sb.String()
	}
	shown := min(len(examples), maxRichExampleRows)
	sb.WriteString(exampleTable(examples[:shown]))
	// The plain card throws the rest away. A collapsible block costs the reader
	// nothing until they want it, and «собака» has thirteen examples in the
	// dictionary against the six that fit a readable message.
	if rest := examples[shown:]; len(rest) > 0 {
		sb.WriteString("<details><summary>" + morePrompt(len(rest)) + "</summary>" +
			exampleTable(rest) + "</details>")
	}
	return sb.String()
}

// exampleTable puts the two languages in two named columns, Chechen always
// first. The plain card separates them with an arrow, which says which way the
// example reads but never which language is on which side — the one thing a
// learner cannot work out from the letters alone.
func exampleTable(examples []example) string {
	rows := make([][2]string, len(examples))
	for i, ex := range examples {
		rows[i] = [2]string{ex.chechen, ex.russian}
	}
	return RichExampleTable(rows)
}

// RichExampleTable renders Chechen/Russian pairs as the two named columns.
// Exported because the grammar card shows set expressions in the same shape:
// the reader learns once that the left column is Chechen, and that holds for
// every table the bot sends.
func RichExampleTable(rows [][2]string) string {
	if len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<table compact><tr><th>чеченский</th><th>русский</th></tr>")
	for _, r := range rows {
		sb.WriteString("<tr><td>" + r[0] + "</td><td>" + r[1] + "</td></tr>")
	}
	sb.WriteString("</table>")
	return sb.String()
}

func morePrompt(n int) string {
	return fmt.Sprintf("Ещё %d %s", n, plural(n, "пример", "примера", "примеров"))
}

// plural picks the Russian form for a count: 1 пример, 2 примера, 5 примеров.
func plural(n int, one, few, many string) string {
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

// FormatRichCard renders a whole lookup: the blocks, the neighbours line and the
// licence footer dosham asks for. <footer> is rendered muted by the client, so
// the attribution can sit on every card instead of only in /start.
func FormatRichCard(query string, pairs []models.TranslationPairs) string {
	r := RenderRich(query, pairs)
	if r.Body == "" {
		return ""
	}
	return r.Body + RichNeighbours(r.Neighbours) + RichFooter
}

const RichFooter = "<footer>Словарь dosham.app</footer>"

func RichNeighbours(neighbours []string) string {
	line := FormatNeighbours(neighbours)
	if line == "" {
		return ""
	}
	return "<p>" + line + "</p>"
}

// RichQuote wraps a remark the bot makes about the lookup itself — «по запросу
// «ваха»», the palochka hint. On the plain card those are italic lines
// indistinguishable from a usage example.
func RichQuote(text string) string { return "<blockquote>" + text + "</blockquote>" }

// display returns the headword as the card shows it: qualifiers peeled off and
// case normalized. The homonym number is left to the caller — the plain card
// keeps it outside the bold, «<b>ваха</b>²». Shared with the plain renderer so
// the two never drift apart.
func (b *block) display() (name string, quals []string) {
	quals, name = splitQualifiers(b.head)
	quals = dropRepeats(quals, b.senses)
	return headCase(name), quals
}

// homonym is the superscript that tells two entries of one spelling apart,
// empty when the word has no homonyms. superscript(1) is «¹», which is why this
// is a method and not a call at each site.
func (b *block) homonym() string {
	if b.index <= 1 {
		return ""
	}
	return superscript(b.index)
}

// senseLines returns the senses as displayed, with the language named where the
// list cannot say it for itself.
//
// One sense is labelled outright. A numbered list is not, because repeating
// «чеченский» down four lines is the pile-up the labels replaced — with one
// exception: a sense spelled like the headword. «телефон» is Russian on the
// headword line and Chechen in the list, and nothing but the label says so.
// That is the case the whole labelling exists for.
func (b *block) senseLines() []string {
	rendered := b.glosses()
	if len(rendered) == 1 {
		return []string{rendered[0] + tag(b.senseLang())}
	}
	_, name := splitQualifiers(b.head)
	key := FoldSearch(trimPunct(name))
	for i := range rendered {
		if FoldSearch(trimPunct(firstVariant(b.senses[i]))) == key {
			rendered[i] += tag(b.senseLang())
		}
	}
	return rendered
}

// glosses returns the senses as displayed, capped and with the Chechen side
// bolded, but without the numbering — the caller decides between «1. » and <li>.
func (b *block) glosses() []string {
	senses := b.senses
	if len(senses) > maxCardSenses {
		senses = senses[:maxCardSenses]
	}
	_, name := splitQualifiers(b.head)
	// Only a one-word entry has one-word glosses; a collocation's translation is
	// a sentence and keeps its capital.
	word := !strings.Contains(strings.TrimSpace(name), " ")
	out := make([]string, 0, len(senses))
	for _, s := range senses {
		quals, rest := splitQualifiers(s)
		if word {
			rest = headCase(rest)
		}
		if !b.cheHead {
			rest = "<b>" + rest + "</b>"
		}
		if len(quals) > 0 {
			rest += " <i>(" + strings.Join(quals, ", ") + ")</i>"
		}
		out = append(out, rest)
	}
	return out
}

// orderedExamples is the block's examples in display order, uncapped.
func (b *block) orderedExamples() []example {
	if !b.cheHead {
		return b.examples
	}
	_, name := splitQualifiers(b.head)
	return freeUsesFirst(b.examples, name)
}

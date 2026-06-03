package business

import (
	"chetoru/internal/models"
	"context"
	"encoding/json"
	"strings"
)

// grammarResponse decodes the dosham `find` query used for grammar lookups.
type grammarResponse struct {
	Data struct {
		Find []grammarEntry `json:"find"`
	} `json:"data"`
}

type grammarEntry struct {
	Content    string        `json:"content"`
	Type       string        `json:"type"`
	Rate       int           `json:"rate"`
	Details    string        `json:"details"`
	EntryForms []grammarForm `json:"entryForms"`
}

type grammarForm struct {
	Content string `json:"content"`
}

// GrammarFor looks up lightweight grammar (part of speech + inflected forms) for
// the Chechen headword most relevant to word. It issues one live `find` query,
// keeps only morphologically analyzed Chechen WORD entries, and picks the
// highest-rated one. Returns nil when there is nothing worth showing, so callers
// can simply skip the grammar card.
func (b *Business) GrammarFor(ctx context.Context, word string) *models.WordGrammar {
	word = strings.TrimSpace(word)
	if word == "" {
		return nil
	}

	best := bestGrammarEntry(b.findGrammarEntries(ctx, word), "")
	if best == nil {
		return nil
	}

	// When the best match for the user's query has no inflected forms, the user
	// likely typed the Russian side and matched a thin translation record. The
	// rich analyzed entry (full paradigm) lives under the Chechen headword, so
	// re-query it directly to enrich. Skip when the user already typed that
	// headword (re-querying the same word can't add anything).
	if len(best.EntryForms) == 0 && !strings.EqualFold(strings.TrimSpace(best.Content), word) {
		if enriched := bestGrammarEntry(b.findGrammarEntries(ctx, best.Content), best.Content); enriched != nil {
			if len(enriched.EntryForms) > 0 || posFromDetails(enriched.Details) != "" {
				best = enriched
			}
		}
	}

	g := &models.WordGrammar{
		Headword: strings.TrimSpace(best.Content),
		POS:      posFromDetails(best.Details),
	}
	for _, f := range best.EntryForms {
		if s := strings.TrimSpace(f.Content); s != "" {
			g.Forms = append(g.Forms, s)
		}
	}
	if g.POS == "" && len(g.Forms) == 0 {
		return nil // nothing worth showing
	}
	return g
}

// findGrammarEntries runs the grammar `find` query and returns the raw entries.
func (b *Business) findGrammarEntries(ctx context.Context, word string) []grammarEntry {
	query := `
		query Grammar($inputText: String!) {
			find(inputText: $inputText) {
				content
				type
				rate
				details
				entryForms { content }
			}
		}
	`
	var resp grammarResponse
	if err := doDoshamQuery(ctx, query, map[string]any{"inputText": word}, &resp); err != nil {
		b.log.Printf("dosham grammar query failed: %v\n", err)
		return nil
	}
	return resp.Data.Find
}

// bestGrammarEntry picks the highest-rated analyzed Chechen WORD entry. When
// mustMatch is non-empty, only entries whose content equals it (case-folded)
// are considered, so an enrichment re-query can't drift to an unrelated word.
func bestGrammarEntry(entries []grammarEntry, mustMatch string) *grammarEntry {
	mustMatch = strings.TrimSpace(mustMatch)
	var best *grammarEntry
	for i := range entries {
		e := &entries[i]
		// Only Chechen headwords are morphologically analyzed: they carry a
		// non-empty `details` blob or a list of inflected forms. Russian
		// headwords have neither, keeping us on the Chechen side of the pair.
		if e.Type != "WORD" || !entryHasGrammar(e) {
			continue
		}
		if mustMatch != "" && !strings.EqualFold(strings.TrimSpace(e.Content), mustMatch) {
			continue
		}
		if best == nil || e.Rate > best.Rate {
			best = e
		}
	}
	return best
}

func entryHasGrammar(e *grammarEntry) bool {
	if d := strings.TrimSpace(e.Details); d != "" && d != "null" {
		return true
	}
	return len(e.EntryForms) > 0
}

// posFromDetails infers a human-readable part of speech from WHICH keys the
// dosham `details` JSON contains. This is safe without the (undocumented)
// integer code legend: the key-set alone distinguishes nouns from verbs. Only
// high-confidence categories are labelled; the adjective/adverb schema is
// ambiguous, so it returns "" rather than risk a wrong label.
func posFromDetails(detailsJSON string) string {
	detailsJSON = strings.TrimSpace(detailsJSON)
	if detailsJSON == "" || detailsJSON == "null" {
		return ""
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(detailsJSON), &keys); err != nil {
		return ""
	}
	has := func(k string) bool { _, ok := keys[k]; return ok }

	switch {
	case has("Tense") || has("Mood") || has("Conjugation") || has("Transitiveness"):
		return "глагол"
	case has("Declension") && has("Case"):
		return "существительное"
	default:
		return ""
	}
}

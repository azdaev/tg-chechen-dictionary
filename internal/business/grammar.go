package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// grammarResponse decodes the dosham `find` query used for grammar lookups.
type grammarResponse struct {
	Data struct {
		Find []grammarEntry `json:"find"`
	} `json:"data"`
}

type grammarEntry struct {
	Content        string         `json:"content"`
	Type           string         `json:"type"`
	Rate           int            `json:"rate"`
	Details        string         `json:"details"`
	EntryForms     []grammarForm  `json:"entryForms"`
	RelatedEntries []relatedEntry `json:"relatedEntries"`
}

type grammarForm struct {
	Content string `json:"content"`
}

type relatedEntry struct {
	Content      string               `json:"content"`
	Translations []grammarTranslation `json:"translations"`
}

type grammarTranslation struct {
	Content      string `json:"content"`
	LanguageCode string `json:"languageCode"`
}

// maxIdioms caps how many set phrases the grammar card carries.
const maxIdioms = 5

// GrammarFor looks up lightweight grammar (part of speech + inflected forms) for
// the Chechen headword most relevant to word. Grammar cards are only meaningful
// for a single word, so multi-word input is skipped without a query. Results
// (including "no grammar") are cached, since the live dosham lookup is otherwise
// repeated on every translation. Returns nil when there is nothing worth showing.
func (b *Business) GrammarFor(ctx context.Context, word string) *models.WordGrammar {
	word = strings.TrimSpace(word)
	if word == "" || strings.ContainsAny(word, " \t\n") {
		return nil
	}

	cacheKey := normalizeCacheKey(word)
	if g, err := b.cache.GetGrammar(ctx, cacheKey); err == nil {
		return g
	} else if !errors.Is(err, cache.ErrMiss) {
		b.log.Printf("grammar cache get failed for %q: %v\n", cacheKey, err)
	}

	g := b.computeGrammar(ctx, word)

	if err := b.cache.SetGrammar(ctx, cacheKey, g); err != nil {
		b.log.Printf("grammar cache set failed for %q: %v\n", cacheKey, err)
	}
	return g
}

// computeGrammar runs the live dosham lookup behind GrammarFor, keeping only a
// morphologically analyzed Chechen WORD entry that exactly matches the user's
// headword. Russian-side translation queries should not produce a second
// grammar message for the Chechen translation.
func (b *Business) computeGrammar(ctx context.Context, word string) *models.WordGrammar {
	best := bestGrammarEntry(b.findGrammarEntries(ctx, word), word)
	if best == nil {
		return nil
	}

	// When the best match for the user's query carries neither a paradigm nor set
	// phrases, the user likely typed the Russian side and matched a thin
	// translation record. The rich analyzed entry lives under the Chechen
	// headword, so re-query it directly to enrich. Skip when the user already
	// typed that headword (re-querying the same word can't add anything).
	if len(best.EntryForms) == 0 && len(best.RelatedEntries) == 0 && !strings.EqualFold(strings.TrimSpace(best.Content), word) {
		if enriched := bestGrammarEntry(b.findGrammarEntries(ctx, best.Content), best.Content); enriched != nil {
			if len(enriched.EntryForms) > 0 || len(enriched.RelatedEntries) > 0 || posFromDetails(enriched.Details) != "" {
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
	for _, r := range best.RelatedEntries {
		che := strings.TrimSpace(r.Content)
		ru := firstRussian(r.Translations)
		if che == "" || ru == "" {
			continue // an idiom without a translation is useless to a learner
		}
		g.Idioms = append(g.Idioms, models.Idiom{Chechen: che, Russian: ru})
		if len(g.Idioms) >= maxIdioms {
			break
		}
	}

	if g.POS == "" && len(g.Forms) == 0 && len(g.Idioms) == 0 {
		return nil // nothing worth showing
	}
	return g
}

// firstRussian returns the first Russian translation content, or "".
func firstRussian(translations []grammarTranslation) string {
	for _, t := range translations {
		if normalizeLang(t.LanguageCode) == "RUS" {
			if s := strings.TrimSpace(t.Content); s != "" {
				return s
			}
		}
	}
	return ""
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
				relatedEntries { content translations { content languageCode } }
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
// mustMatch is non-empty, only entries whose normalized content equals it are
// considered, so palochka stand-ins still match but substring hits do not.
func bestGrammarEntry(entries []grammarEntry, mustMatch string) *grammarEntry {
	mustMatch = tools.NormalizeSearch(mustMatch)
	var best *grammarEntry
	for i := range entries {
		e := &entries[i]
		// Only Chechen headwords are morphologically analyzed: they carry a
		// non-empty `details` blob or a list of inflected forms. Russian
		// headwords have neither, keeping us on the Chechen side of the pair.
		if e.Type != "WORD" || !entryHasGrammar(e) {
			continue
		}
		if mustMatch != "" && tools.NormalizeSearch(e.Content) != mustMatch {
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

package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	Details string `json:"details"`
}

// morphology is the part of dosham's `details` the card names. The codes are
// dosham's own enums; the names below are the ones dosham.app itself shows
// (its frontend bundle and assets/i18n/ru.json, read 2026-11-06).
type morphology struct {
	Class       int
	Case        int
	NumeralType int
}

func parseMorphology(details string) morphology {
	var m morphology
	_ = json.Unmarshal([]byte(details), &m) // missing or broken details name nothing
	return m
}

// nounClass is the singular agreement of each class. dosham labels the codes
// 1 «в, б/д», 2 «й, б/д», 3 «й, й», 4 «д, д», 5 «б, б/й», 6 «б, д».
var nounClass = map[int]string{1: "ву", 2: "йу", 3: "йу", 4: "ду", 5: "бу", 6: "бу"}

var caseNames = map[int]string{
	1: "именительный", 2: "родительный", 3: "дательный", 4: "эргативный",
	5: "вещественный", 6: "творительный", 7: "местный", 8: "аллативный",
	9: "аблативный", 10: "пролативный", 11: "сравнительный",
}

const pluralNumber = 2

// caseName names a form: «родительный», «множественное» for the plural
// nominative, «множественное, родительный» for the rest of the plural.
func (m morphology) caseName() string {
	name := caseNames[m.Case]
	if m.NumeralType != pluralNumber {
		return name
	}
	if name == "" || m.Case == 1 {
		return "множественное"
	}
	return "множественное, " + name
}

// order sorts a paradigm the way grammars print it: the singular cases in
// dosham's enum order, which is the textbook one, then the plural, then forms
// dosham did not label.
func (m morphology) order() int {
	if m.Case == 0 && m.NumeralType != pluralNumber {
		return 1000
	}
	if m.NumeralType == pluralNumber {
		return 100 + m.Case
	}
	return m.Case
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
// maxIdioms caps the set phrases a grammar card carries. Examples are the
// scarce thing in Chechen — dosham holds 20 for «куьг» and 38 for «хи» — and
// they cost nothing extra: the paradigm query already returned them.
// Cached entries keep the old count until their thirty days run out; the key is
// deliberately unversioned, because bumping it would spend one live dosham query
// per word to gain three lines.
const maxIdioms = 8

// GrammarFor looks up lightweight grammar (part of speech + inflected forms) for
// the Chechen headword most relevant to word. Grammar cards are only meaningful
// for a single word, so multi-word input is skipped without a query. Results
// (including "no grammar") are cached, since the live dosham lookup is otherwise
// repeated on every translation. Returns nil when there is nothing worth showing.
//
// A non-nil error means the lookup failed, not that the word has no grammar.
// The distinction has to survive the cache: grammar entries live for 30 days,
// so writing an outage down as "no grammar" would hide the paradigm for a month.
func (b *Business) GrammarFor(ctx context.Context, word string) (*models.WordGrammar, error) {
	word = strings.TrimSpace(word)
	if word == "" || strings.ContainsAny(word, " \t\n") {
		return nil, nil
	}

	cacheKey := normalizeCacheKey(word)
	if g, err := b.cache.GetGrammar(ctx, cacheKey); err == nil {
		// Recorded on the cache hit too, not just on compute: the grammar cache
		// holds thirty days, so skipping it here would leave exactly the
		// most-looked-up words — the already-cached ones — without a paradigm
		// for a month. ponytail: a few no-op inserts per lookup, detached; give
		// it a seen-set if the write rate ever shows up in the logs.
		if g != nil {
			b.saveWordForms(g.Headword, g.Forms)
		}
		return g, nil
	} else if !errors.Is(err, cache.ErrMiss) {
		b.log.Printf("grammar cache get failed for %q: %v\n", cacheKey, err)
	}

	g, err := b.computeGrammar(ctx, word)
	if err != nil {
		return nil, err
	}
	if g != nil {
		b.saveWordForms(g.Headword, g.Forms)
	}

	if err := b.cache.SetGrammar(ctx, cacheKey, g); err != nil {
		b.log.Printf("grammar cache set failed for %q: %v\n", cacheKey, err)
	}
	return g, nil
}

// computeGrammar runs the live dosham lookup behind GrammarFor: one `find` query,
// keeping only morphologically analyzed Chechen WORD entries, picking the
// highest-rated, and enriching cross-direction matches.
func (b *Business) computeGrammar(ctx context.Context, word string) (*models.WordGrammar, error) {
	entries, err := b.findGrammarEntries(ctx, word)
	if err != nil {
		return nil, err
	}
	// The headword the user is looking at gets first refusal, then the
	// highest-rated analyzed entry. It has to be a preference and not a filter:
	// bestGrammarEntry already considers only analyzed Chechen entries, so
	// requiring a match would return nil for every Russian query — the grammar
	// card would vanish for half the dictionary rather than pick the wrong word.
	best := bestGrammarEntry(entries, word)
	if best == nil {
		best = bestGrammarEntry(entries, "")
	}
	if best == nil {
		return nil, nil
	}

	// When the best match for the user's query carries neither a paradigm nor set
	// phrases, the user likely typed the Russian side and matched a thin
	// translation record. The rich analyzed entry lives under the Chechen
	// headword, so re-query it directly to enrich. Skip when the user already
	// typed that headword (re-querying the same word can't add anything).
	if len(best.EntryForms) == 0 && len(best.RelatedEntries) == 0 && !strings.EqualFold(strings.TrimSpace(best.Content), word) {
		// Enrichment only, as long as the thin entry we already have says
		// something: a part of speech is a real answer and safe to cache.
		found, findErr := b.findGrammarEntries(ctx, best.Content)
		enriched := bestGrammarEntry(found, best.Content)
		switch {
		case enriched != nil && (len(enriched.EntryForms) > 0 || len(enriched.RelatedEntries) > 0 || posFromDetails(enriched.Details) != ""):
			best = enriched
		case findErr != nil && posFromDetails(best.Details) == "":
			// This entry has no forms and no idioms by the branch condition, so
			// with no part of speech either it renders nothing and falls out
			// below as "no grammar" — cached for thirty days. The query that
			// would have filled it is the one that just failed, and an outage
			// must not be written down as an answer.
			return nil, findErr
		}
	}

	g := &models.WordGrammar{
		Headword: strings.TrimSpace(best.Content),
		POS:      posFromDetails(best.Details),
	}
	// dosham shows a class only on nouns, and so does the card.
	if g.POS == "существительное" {
		g.Class = nounClass[parseMorphology(best.Details).Class]
	}
	forms := slices.Clone(best.EntryForms)
	slices.SortStableFunc(forms, func(a, b grammarForm) int {
		return parseMorphology(a.Details).order() - parseMorphology(b.Details).order()
	})
	for _, f := range forms {
		if s := strings.TrimSpace(f.Content); s != "" {
			g.Forms = append(g.Forms, s)
			g.Cases = append(g.Cases, parseMorphology(f.Details).caseName())
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
		return nil, nil // nothing worth showing
	}
	return g, nil
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
func (b *Business) findGrammarEntries(ctx context.Context, word string) ([]grammarEntry, error) {
	query := `
		query Grammar($inputText: String!) {
			find(inputText: $inputText) {
				content
				type
				rate
				details
				entryForms { content details }
				relatedEntries { content translations { content languageCode } }
			}
		}
	`
	var resp grammarResponse
	if err := doDoshamQuery(ctx, query, map[string]any{"inputText": word}, &resp); err != nil {
		return nil, fmt.Errorf("dosham grammar %q: %w", word, err)
	}
	return resp.Data.Find, nil
}

// bestGrammarEntry picks the highest-rated analyzed Chechen WORD entry. When
// mustMatch is non-empty, only entries whose headword matches it are
// considered, so a caller can pin the result to one word — an enrichment
// re-query must not drift, and callers that merely prefer a word retry with an
// empty mustMatch.
//
// Matching goes through the search normalization, not EqualFold: displayed
// headwords differ from what reaches here by ё, by which character stands in
// for the palochka, and by combining stress marks.
func bestGrammarEntry(entries []grammarEntry, mustMatch string) *grammarEntry {
	key := tools.NormalizeSearch(mustMatch)
	var best *grammarEntry
	for i := range entries {
		e := &entries[i]
		// Only Chechen headwords are morphologically analyzed: they carry a
		// non-empty `details` blob or a list of inflected forms. Russian
		// headwords have neither, keeping us on the Chechen side of the pair.
		if e.Type != "WORD" || !entryHasGrammar(e) {
			continue
		}
		if key != "" && tools.NormalizeSearch(e.Content) != key {
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

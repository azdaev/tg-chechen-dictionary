package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"

	"context"
	"fmt"
	"strings"
)

// RandomWordFromAPI returns one clean Chechen → Russian pair drawn from the
// full dictionary (130K+ entries), so /random works even when the local
// moderated table is empty. Pairs come from the prefetched word pool, so the
// common case costs no API call. On error the caller falls back to the local
// moderated table.
func (b *Business) RandomWordFromAPI(ctx context.Context) (*models.RandomWord, error) {
	words, err := b.randomCleanWords(ctx, 1)
	if err != nil {
		return nil, err
	}
	return &words[0], nil
}

// orientEntry turns a dictionary entry into a Chechen → Russian pair. An entry's
// content is the headword in one language and its translations are in others, so
// we infer direction from the translation's language code. Prefers the Russian
// reading: there the content is the Chechen headword, which is a clean single
// word, whereas the Chechen reading often yields a long dictionary gloss.
// Returns nil if no Russian/Chechen pairing exists.
func orientEntry(entry models.Entry) *models.RandomWord {
	for _, t := range entry.Translations {
		if normalizeLang(t.LanguageCode) == "RUS" {
			// content is Chechen, translation is the Russian meaning
			return makeRandomWord(entry.Content, t.Content)
		}
	}
	for _, t := range entry.Translations {
		if normalizeLang(t.LanguageCode) == "CHE" {
			// content is Russian, translation is the Chechen word
			return makeRandomWord(t.Content, entry.Content)
		}
	}
	return nil
}

// isLearnableWord reports whether a Chechen string is a clean standalone word
// suitable for a discovery card — not a multi-clause dictionary gloss with
// examples, grammatical markers, or numbered senses.
func isLearnableWord(chechen string) bool {
	chechen = strings.TrimSpace(chechen)
	if chechen == "" {
		return false
	}
	// Dictionary glosses carry sense numbers, tildes, parens, semicolons,
	// commas, or abbreviation periods ("нареч.", "см.", "понуд. от …"); a
	// leading "-" marks a grammatical-ending entry ("-ая, -ое …"). A clean
	// learnable word has none of these.
	if strings.ContainsAny(chechen, ";,.~()[]1234567890") {
		return false
	}
	if strings.HasPrefix(chechen, "-") {
		return false
	}
	if strings.Count(chechen, " ") > 1 {
		return false
	}
	return len([]rune(chechen)) <= 24
}

func makeRandomWord(chechen, russian string) *models.RandomWord {
	chechen = stripLeadingGenderMarker(strings.TrimSpace(tools.Clean(chechen)))
	russian = strings.TrimSpace(tools.Clean(russian))
	if chechen == "" || russian == "" {
		return nil
	}
	return &models.RandomWord{Chechen: stripStressMarks(chechen), Russian: stripStressMarks(russian)}
}

// stripStressMarks drops combining acute accents ("совеща́ние") — dictionary
// stress metadata that renders unevenly in chat clients and makes accented
// and plain spellings look like different words to dedup and answer matching.
func stripStressMarks(s string) string {
	if !strings.ContainsRune(s, '\u0301') {
		return s
	}
	return strings.ReplaceAll(s, "\u0301", "")
}

// genderMarkers are the Russian grammatical-gender/number abbreviations that can
// prefix a translation in the source data (м=мужской, ж=женский, с=средний,
// мн=множественное). They are not part of the Chechen word.
var genderMarkers = []string{"мн", "м", "ж", "с"}

// stripLeadingGenderMarker removes a single standalone Russian gender/number
// marker from the head of a Chechen card ("ж астагӏалла" → "астагӏалла"). It
// only strips when a real word follows, and never the whole string, so a
// genuine one-word card is left untouched.
func stripLeadingGenderMarker(chechen string) string {
	for _, m := range genderMarkers {
		if rest, ok := strings.CutPrefix(chechen, m+" "); ok {
			if rest = strings.TrimSpace(rest); rest != "" {
				return rest
			}
		}
	}
	return chechen
}

// fetchRandomEntries asks the dosham API for a batch of random dictionary
// entries. Shared by the /random and /quiz features.
func (b *Business) fetchRandomEntries(ctx context.Context, count int) ([]models.Entry, error) {
	query := fmt.Sprintf(`{ randomEntries(count: %d) { content type translations { content languageCode } } }`, count)

	var response struct {
		Data struct {
			RandomEntries []models.Entry `json:"randomEntries"`
		} `json:"data"`
	}
	if err := doDoshamQuery(ctx, query, nil, &response); err != nil {
		return nil, err
	}

	return response.Data.RandomEntries, nil
}

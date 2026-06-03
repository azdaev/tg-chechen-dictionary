package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RandomWordFromAPI fetches a batch of random entries from the dosham API and
// returns one oriented as Chechen → Russian. It draws from the full dictionary
// (130K+ entries), so /random works even when the local moderated table is
// empty. Single-word ("WORD") entries are preferred over phrase ("TEXT")
// entries for a cleaner learning card. Returns nil if nothing usable was found.
func (b *Business) RandomWordFromAPI(ctx context.Context) (*models.RandomWord, error) {
	entries, err := b.fetchRandomEntries(ctx, 20)
	if err != nil {
		return nil, err
	}

	// Pick the best card in the batch: a single WORD entry whose Chechen side is
	// a clean, learnable word. Fall back progressively to any WORD, then anything.
	var bestWord, anyWord, fallback *models.RandomWord
	for _, entry := range entries {
		word := orientEntry(entry)
		if word == nil {
			continue
		}
		if fallback == nil {
			fallback = word
		}
		if entry.Type != "WORD" {
			continue
		}
		if anyWord == nil {
			anyWord = word
		}
		if bestWord == nil && isLearnableWord(word.Chechen) {
			bestWord = word
		}
	}

	switch {
	case bestWord != nil:
		return bestWord, nil
	case anyWord != nil:
		return anyWord, nil
	default:
		return fallback, nil
	}
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
	if strings.ContainsAny(chechen, ";~()1234567890") {
		return false
	}
	if strings.Count(chechen, " ") > 1 {
		return false
	}
	return len([]rune(chechen)) <= 24
}

func makeRandomWord(chechen, russian string) *models.RandomWord {
	chechen = strings.TrimSpace(tools.Clean(chechen))
	russian = strings.TrimSpace(tools.Clean(russian))
	if chechen == "" || russian == "" {
		return nil
	}
	return &models.RandomWord{Chechen: chechen, Russian: russian}
}

// fetchRandomEntries asks the dosham API for a batch of random dictionary
// entries. Shared by the /random and /quiz features.
func (b *Business) fetchRandomEntries(ctx context.Context, count int) ([]models.Entry, error) {
	requestBody := map[string]interface{}{
		"query": fmt.Sprintf(`{ randomEntries(count: %d) { content type translations { content languageCode } } }`, count),
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", doshamAPIURL(), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response struct {
		Data struct {
			RandomEntries []models.Entry `json:"randomEntries"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Data.RandomEntries, nil
}

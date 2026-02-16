package main

import (
	"bytes"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

const apiURL = "https://v2.api.chldr.movsar.dev/v2/graphql/"

type apiResponse struct {
	Data struct {
		Find struct {
			Success        bool   `json:"success"`
			SerializedData string `json:"serializedData"`
			ErrorMessage   string `json:"errorMessage"`
		} `json:"find"`
	} `json:"data"`
}

func main() {
	var (
		dbPath   string
		wordlist string
		source   string
	)
	flag.StringVar(&dbPath, "db", "./database.db", "SQLite database path")
	flag.StringVar(&wordlist, "wordlist", "", "Path to newline-delimited wordlist")
	flag.StringVar(&source, "source", "api", "Source label for imported pairs")
	flag.Parse()

	if wordlist == "" {
		fmt.Fprintln(os.Stderr, "wordlist is required")
		os.Exit(1)
	}

	words, err := readWordlist(wordlist)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read wordlist:", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := repository.NewRepository(db)
	ctx := context.Background()

	client := &http.Client{}
	for _, word := range words {
		entries, err := fetchEntries(client, word)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch failed for %q: %v\n", word, err)
			continue
		}

		var processEntry func(entry models.Entry)
		processEntry = func(entry models.Entry) {
			for _, translation := range entry.Translations {
				if translation.LanguageCode != "RUS" && translation.LanguageCode != "CHE" {
					continue
				}

				originalLang := inferOriginalLang(translation.LanguageCode)
				if originalLang == "" {
					continue
				}

				originalRaw := strings.TrimSpace(entry.Content)
				translationRaw := strings.TrimSpace(translation.Content)
				if originalRaw == "" || translationRaw == "" {
					continue
				}

				pair := repository.TranslationPair{
					OriginalRaw:         originalRaw,
					OriginalClean:       normalizeText(originalRaw),
					OriginalLang:        originalLang,
					TranslationRaw:      translationRaw,
					TranslationClean:    normalizeText(translationRaw),
					TranslationLang:     translation.LanguageCode,
					Source:              source,
					SourceEntryID:       toNullString(entry.EntryID),
					SourceTranslationID: toNullString(translation.TranslationID),
					IsApproved:          false,
				}

				if _, err := repo.InsertTranslationPair(ctx, pair); err != nil {
					fmt.Fprintf(os.Stderr, "insert failed for %q: %v\n", word, err)
				}
			}

			for _, sub := range entry.SubEntries {
				processEntry(sub)
			}
		}

		for _, entry := range entries {
			processEntry(entry)
		}
	}
}

func readWordlist(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	words := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		words = append(words, line)
	}
	return words, nil
}

func fetchEntries(client *http.Client, word string) ([]models.Entry, error) {
	query := `
		query Find($inputText: String!) {
			find(inputText: $inputText) {
				success
				serializedData
				errorMessage
			}
		}
	`

	requestBody := map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"inputText": word,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if !response.Data.Find.Success {
		return nil, fmt.Errorf("api error: %s", response.Data.Find.ErrorMessage)
	}

	var entries []models.Entry
	if err := json.Unmarshal([]byte(response.Data.Find.SerializedData), &entries); err != nil {
		return nil, err
	}

	return entries, nil
}

func normalizeText(text string) string {
	clean := tools.Clean(text)
	clean = strings.TrimSpace(clean)
	clean = strings.ToLower(clean)
	return clean
}

func inferOriginalLang(translationLang string) string {
	switch translationLang {
	case "RUS":
		return "CHE"
	case "CHE":
		return "RUS"
	default:
		return ""
	}
}

func toNullString(v string) sql.NullString {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

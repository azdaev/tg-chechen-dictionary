package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"sort"
	"unicode/utf8"

	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Business struct {
	cache *cache.Cache
	log   *logrus.Logger
}

func NewBusiness(cache *cache.Cache, log *logrus.Logger) *Business {
	return &Business{
		cache: cache,
		log:   log,
	}
}

func (b *Business) Translate(word string) []models.TranslationPairs {
	// Пробуем получить перевод из кэша
	translations, err := b.cache.Get(context.Background(), word)
	if err == nil {
		return translations
	}

	// Если в кэше нет, делаем запрос к API
	query := `
		query Find($inputText: String!) {
			find(inputText: $inputText) {
				success
				serializedData
				errorMessage
			}
		}
	`

	variables := map[string]interface{}{
		"inputText": word,
	}

	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		b.log.Printf("failed to marshal request: %v\n", err)
		return nil
	}

	req, err := http.NewRequest("POST", "https://api.dosham.app/v2/graphql/", bytes.NewBuffer(jsonData))
	if err != nil {
		b.log.Printf("failed to create request: %v\n", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		b.log.Printf("failed to do request: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	var response models.TranslationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		b.log.Printf("failed to decode response: %v\n", err)
		return nil
	}

	if !response.Data.Find.Success {
		b.log.Printf("API request failed: %s\n", response.Data.Find.ErrorMessage)
		return nil
	}

	// Parse the serialized data which contains the entries
	var entries []models.Entry
	if err := json.Unmarshal([]byte(response.Data.Find.SerializedData), &entries); err != nil {
		b.log.Printf("failed to unmarshal serialized data: %v\n", err)
		return nil
	}

	translations = make([]models.TranslationPairs, 0)

	// Process entries and their subentries
	var processEntry func(entry models.Entry)
	processEntry = func(entry models.Entry) {
		// Process translations for the current entry
		for _, translation := range entry.Translations {
			// We're looking for translations in Russian (RUS) and Chechen (CHE)
			if translation.LanguageCode == "RUS" || translation.LanguageCode == "CHE" {
				translationPair := models.TranslationPairs{
					Original:  entry.Content,
					Translate: translation.Content,
				}
				translationPair.Original = tools.EscapeUnclosedTags(translationPair.Original)
				translationPair.Translate = tools.EscapeUnclosedTags(translationPair.Translate)
				translations = append(translations, translationPair)
			}
		}

		// Process subentries recursively
		for _, subentry := range entry.SubEntries {
			processEntry(subentry)
		}
	}

	// Process all entries
	for _, entry := range entries {
		processEntry(entry)
	}

	if utf8.RuneCountInString(word) <= 3 && len(translations) >= 10 {
		translations = translations[:10]
	}

	// Sort translations by length of the original word (shortest to longest)
	sort.Slice(translations, func(i, j int) bool {
		return utf8.RuneCountInString(translations[i].Original) < utf8.RuneCountInString(translations[j].Original)
	})

	// Сохраняем результат в кэш
	go func() {
		err = b.cache.Set(context.Background(), word, translations)
		if err != nil {
			b.log.Printf("failed to cache translation: %v\n", err)
		}
	}()

	return translations
}

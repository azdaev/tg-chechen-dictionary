package business

import (
	"chetoru/internal/ai"
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"chetoru/pkg/tools"
	"sort"
	"unicode/utf8"

	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Business struct {
	cache    *cache.Cache
	dictRepo DictionaryRepository
	aiClient *ai.Client // optional, can be nil
	log      *logrus.Logger
}

type DictionaryRepository interface {
	FindApprovedTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	InsertTranslationPair(ctx context.Context, pair repository.TranslationPair) (int64, error)
	UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error
}

func NewBusiness(cache *cache.Cache, dictRepo DictionaryRepository, aiClient *ai.Client, log *logrus.Logger) *Business {
	return &Business{
		cache:    cache,
		dictRepo: dictRepo,
		aiClient: aiClient,
		log:      log,
	}
}

func (b *Business) Translate(word string) []models.TranslationPairs {
	cacheKey := tools.NormalizeSearch(word)
	// Пробуем получить перевод из кэша
	translations, err := b.cache.Get(context.Background(), cacheKey)
	if err == nil {
		return translations
	}

	// Пробуем получить перевод из локальной базы
	if b.dictRepo != nil {
		cleanWord := tools.NormalizeSearch(word)
		if cleanWord != "" {
			localTranslations, err := b.dictRepo.FindApprovedTranslationPairs(context.Background(), cleanWord, 200)
			if err != nil {
				b.log.Printf("failed to read dictionary pairs: %v\n", err)
			} else if len(localTranslations) > 0 {
				go func() {
					err = b.cache.Set(context.Background(), cacheKey, localTranslations)
					if err != nil {
						b.log.Printf("failed to cache translation: %v\n", err)
					}
				}()
				return localTranslations
			}
		}
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

				if b.dictRepo != nil {
					originalLang := inferOriginalLang(translation.LanguageCode)
					if originalLang != "" {
						pair := repository.TranslationPair{
							OriginalRaw:         strings.TrimSpace(entry.Content),
							OriginalClean:       normalizeText(entry.Content),
							OriginalLang:        originalLang,
							TranslationRaw:      strings.TrimSpace(translation.Content),
							TranslationClean:    normalizeText(translation.Content),
							TranslationLang:     translation.LanguageCode,
							Source:              "api",
							SourceEntryID:       toNullString(entry.EntryID),
							SourceTranslationID: toNullString(translation.TranslationID),
							IsApproved:          true,
						}
						if pair.OriginalClean != "" && pair.TranslationClean != "" {
							pairID, err := b.dictRepo.InsertTranslationPair(context.Background(), pair)
							if err != nil {
								b.log.Printf("failed to insert dictionary pair: %v\n", err)
							} else if b.aiClient != nil && pairID > 0 {
								// Asynchronously format with AI
								go b.formatPairWithAI(pairID, pair.OriginalRaw, pair.TranslationRaw)
							}
						}
					}
				}
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
		err = b.cache.Set(context.Background(), cacheKey, translations)
		if err != nil {
			b.log.Printf("failed to cache translation: %v\n", err)
		}
	}()

	return translations
}

func normalizeText(text string) string {
	return tools.NormalizeSearch(text)
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

// TranslateFormatted возвращает переводы с отформатированным текстом, используя кэширование
func (b *Business) TranslateFormatted(word string) *models.TranslationResult {
	cacheKey := tools.NormalizeSearch(word)
	// Пробуем получить отформатированный результат из кэша
	result, err := b.cache.GetTranslationResult(context.Background(), cacheKey)
	if err == nil {
		return result
	}

	// Если в кэше нет, получаем переводы обычным способом
	translations := b.Translate(word)
	if len(translations) == 0 {
		return &models.TranslationResult{
			Pairs:     []models.TranslationPairs{},
			Formatted: "",
		}
	}

	// Форматируем результат
	var formattedResult string
	for _, t := range translations {
		// Используем новое форматирование если перевод содержит словарную статью
		// Признаки сложного перевода: нумерация (1), 2)) или тильды (~)
		isComplexTranslation := strings.Contains(t.Translate, "1)") ||
			strings.Contains(t.Translate, "2)") ||
			strings.Contains(t.Translate, "~") ||
			strings.Contains(t.Original, "1)") ||
			strings.Contains(t.Original, "2)") ||
			strings.Contains(t.Original, "~")

		if isComplexTranslation {
			// Определяем, какое поле содержит сложный перевод
			if strings.Contains(t.Translate, "1)") || strings.Contains(t.Translate, "2)") || strings.Contains(t.Translate, "~") {
				// Создаем словарную статью в нужном формате
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Original, t.Translate)
				formatted := tools.FormatTranslationLite(dictionaryEntry)
				formattedResult += formatted + "\n\n"
			} else if strings.Contains(t.Original, "1)") || strings.Contains(t.Original, "2)") || strings.Contains(t.Original, "~") {
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Translate, t.Original)
				formatted := tools.FormatTranslationLite(dictionaryEntry)
				formattedResult += formatted + "\n\n"
			}
		} else {
			// Обычное форматирование для простых переводов
			formattedResult += fmt.Sprintf("%s — %s\n\n", t.Original, tools.Clean(t.Translate))
		}
	}

	result = &models.TranslationResult{
		Pairs:     translations,
		Formatted: strings.TrimSpace(formattedResult),
	}

	// Сохраняем отформатированный результат в кэш
	go func() {
		err = b.cache.SetTranslationResult(context.Background(), cacheKey, result)
		if err != nil {
			b.log.Printf("failed to cache formatted translation: %v\n", err)
		}
	}()

	return result
}

// formatPairWithAI asynchronously formats a dictionary pair using AI and saves it to DB
func (b *Business) formatPairWithAI(pairID int64, originalRaw, translationRaw string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build raw entry for formatting
	rawEntry := fmt.Sprintf("**%s** - %s", originalRaw, translationRaw)

	// Format with AI
	formatted, err := b.aiClient.FormatDictionaryEntry(ctx, rawEntry)
	if err != nil {
		b.log.Printf("ai formatting failed for pair %d: %v\n", pairID, err)
		return
	}

	// Save to database
	if err := b.dictRepo.UpdateTranslationPairFormatting(ctx, pairID, formatted, ""); err != nil {
		b.log.Printf("failed to save ai formatting for pair %d: %v\n", pairID, err)
	} else {
		b.log.Printf("successfully formatted pair %d with AI\n", pairID)
	}
}

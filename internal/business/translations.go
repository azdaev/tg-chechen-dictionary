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

// OnPairReady is called after a new pair is saved and AI formatting completes (or is skipped).
// pairID is the database ID, cleanWord is the normalized search term.
type OnPairReady func(pairID int64, cleanWord string)

type Business struct {
	cache       *cache.Cache
	dictRepo    DictionaryRepository
	aiClient    *ai.Client // optional, can be nil
	log         *logrus.Logger
	onPairReady OnPairReady
}

type DictionaryRepository interface {
	FindApprovedTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	InsertTranslationPair(ctx context.Context, pair repository.TranslationPair) (int64, error)
	UpdateTranslationPairFormatting(ctx context.Context, id int64, formattedAI, formattedChosen string) error
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string, approved bool, approvedBy string) error
}

func NewBusiness(cache *cache.Cache, dictRepo DictionaryRepository, aiClient *ai.Client, log *logrus.Logger) *Business {
	return &Business{
		cache:    cache,
		dictRepo: dictRepo,
		aiClient: aiClient,
		log:      log,
	}
}

// SetOnPairReady sets a callback that fires after a pair is saved and AI-formatted.
func (b *Business) SetOnPairReady(fn OnPairReady) {
	b.onPairReady = fn
}

func (b *Business) Translate(word string) []models.TranslationPairs {
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	if translations := b.loadCachedTranslations(ctx, cacheKey); len(translations) > 0 {
		return translations
	}

	if translations := b.loadLocalTranslations(ctx, word); len(translations) > 0 {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
		return translations
	}

	translations := b.fetchTranslationsWithFallback(word)
	if len(translations) > 0 {
		b.cacheTranslationsAsync(ctx, cacheKey, translations)
	}

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
	ctx := context.Background()
	cacheKey := normalizeCacheKey(word)
	result := b.loadCachedFormatted(ctx, cacheKey)
	if result != nil {
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
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Original)
				formattedResult += formatted + "\n\n"
			} else if strings.Contains(t.Original, "1)") || strings.Contains(t.Original, "2)") || strings.Contains(t.Original, "~") {
				dictionaryEntry := fmt.Sprintf("**%s** - %s", t.Translate, t.Original)
				formatted := tools.FormatTranslationLite(dictionaryEntry, t.Translate)
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

	if len(result.Pairs) > 0 {
		b.cacheFormattedAsync(ctx, cacheKey, result)
	}

	return result
}

func (b *Business) fetchTranslationsWithFallback(word string) []models.TranslationPairs {
	word = strings.TrimSpace(word)
	translations := b.fetchTranslationsFromAPI(word)
	if len(translations) > 0 {
		return translations
	}

	altWord := tools.AlternateYo(word)
	if altWord == "" {
		return nil
	}

	return b.fetchTranslationsFromAPI(altWord)
}

func (b *Business) fetchTranslationsFromAPI(word string) []models.TranslationPairs {
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

	translations := make([]models.TranslationPairs, 0)

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

				b.storeTranslationPair(entry, translation)
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

	return translations
}

func normalizeCacheKey(word string) string {
	return tools.NormalizeSearch(word)
}

func (b *Business) loadCachedTranslations(ctx context.Context, cacheKey string) []models.TranslationPairs {
	translations, err := b.cache.Get(ctx, cacheKey)
	if err != nil || len(translations) == 0 {
		return nil
	}
	return translations
}

func (b *Business) cacheTranslationsAsync(ctx context.Context, cacheKey string, translations []models.TranslationPairs) {
	if len(translations) == 0 {
		return
	}
	go func() {
		if err := b.cache.Set(ctx, cacheKey, translations); err != nil {
			b.log.Printf("failed to cache translation: %v\n", err)
		}
	}()
}

func (b *Business) loadLocalTranslations(ctx context.Context, word string) []models.TranslationPairs {
	if b.dictRepo == nil {
		return nil
	}
	cleanWord := tools.NormalizeSearch(word)
	if cleanWord == "" {
		return nil
	}
	translations, err := b.dictRepo.FindApprovedTranslationPairs(ctx, cleanWord, 200)
	if err != nil {
		b.log.Printf("failed to read dictionary pairs: %v\n", err)
		return nil
	}
	return translations
}

func (b *Business) loadCachedFormatted(ctx context.Context, cacheKey string) *models.TranslationResult {
	result, err := b.cache.GetTranslationResult(ctx, cacheKey)
	if err != nil || len(result.Pairs) == 0 {
		return nil
	}
	return result
}

func (b *Business) cacheFormattedAsync(ctx context.Context, cacheKey string, result *models.TranslationResult) {
	if result == nil || len(result.Pairs) == 0 {
		return
	}
	go func() {
		if err := b.cache.SetTranslationResult(ctx, cacheKey, result); err != nil {
			b.log.Printf("failed to cache formatted translation: %v\n", err)
		}
	}()
}

func (b *Business) storeTranslationPair(entry models.Entry, translation models.Translation) {
	if b.dictRepo == nil {
		return
	}

	originalLang := inferOriginalLang(translation.LanguageCode)
	if originalLang == "" {
		return
	}

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
		IsApproved:          false,
	}
	if pair.OriginalClean == "" || pair.TranslationClean == "" {
		return
	}

	pairID, err := b.dictRepo.InsertTranslationPair(context.Background(), pair)
	if err != nil {
		b.log.Printf("failed to insert dictionary pair: %v\n", err)
		return
	}

	if b.aiClient != nil && pairID > 0 {
		go b.formatPairWithAI(pairID, pair.OriginalClean, pair.OriginalRaw, pair.TranslationRaw)
	} else if b.onPairReady != nil && pairID > 0 {
		// No AI client — trigger moderation immediately
		go b.onPairReady(pairID, pair.OriginalClean)
	}
}

// formatPairWithAI asynchronously formats a dictionary pair using AI, saves it, then triggers moderation.
func (b *Business) formatPairWithAI(pairID int64, cleanWord, originalRaw, translationRaw string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build raw entry for formatting
	rawEntry := fmt.Sprintf("**%s** - %s", originalRaw, translationRaw)

	// Format with AI
	formatted, err := b.aiClient.FormatDictionaryEntry(ctx, rawEntry)
	if err != nil {
		b.log.Printf("ai formatting failed for pair %d: %v\n", pairID, err)
	} else {
		// Save to database
		if err := b.dictRepo.UpdateTranslationPairFormatting(ctx, pairID, formatted, ""); err != nil {
			b.log.Printf("failed to save ai formatting for pair %d: %v\n", pairID, err)
		} else {
			b.log.Printf("successfully formatted pair %d with AI\n", pairID)
		}
	}

	// Trigger moderation after AI formatting (or failed attempt)
	if b.onPairReady != nil {
		b.onPairReady(pairID, cleanWord)
	}
}

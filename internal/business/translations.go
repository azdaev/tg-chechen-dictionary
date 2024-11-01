package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"
	"unicode/utf8"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"

	"chetoru/internal/cache"
)

type Business struct {
	cache *cache.Cache
}

func NewBusiness(cache *cache.Cache) *Business {
	return &Business{
		cache: cache,
	}
}

func (b *Business) Translate(word string) []models.TranslationPairs {
	// Пробуем получить перевод из кэша
	translations, err := b.cache.Get(context.Background(), word)
	if err == nil {
		return translations
	}

	// Если в кэше нет, делаем запрос к API
	values := map[string]string{
		"word": word,
	}

	data := new(bytes.Buffer)
	formWriter := multipart.NewWriter(data)
	for key, value := range values {
		_ = formWriter.WriteField(key, value)
	}
	formWriter.Close()

	req, err := http.NewRequest("POST", "https://ps95.ru/dikdosham/backend/get_translate.php", data)
	if err != nil {
		fmt.Println(err)
	}
	req.Header.Set("Content-Type", formWriter.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()

	var response models.TranslationResponse
	json.NewDecoder(resp.Body).Decode(&response)
	translations = make([]models.TranslationPairs, 0)

	for _, dict := range response.Data {
		for _, word := range dict.Words {
			translation := models.TranslationPairs{
				Original:  word.Word,
				Translate: word.Translate,
			}
			translation.Original = tools.EscapeUnclosedTags(translation.Original)
			translation.Translate = tools.EscapeUnclosedTags(translation.Translate)
			translations = append(translations, translation)
		}

		if utf8.RuneCountInString(word) < 3 && len(translations) >= 30 {
			break
		}
	}

	// Сохраняем результат в кэш
	err = b.cache.Set(context.Background(), word, translations)
	if err != nil {
		fmt.Printf("failed to cache translation: %v\n", err)
	}

	return translations
}

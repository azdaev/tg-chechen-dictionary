package business

import (
	"chetoru/internal/models"
	"chetoru/pkg/tools"

	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
)

type Business struct{}

func NewBusiness() *Business {
	return &Business{}
}

func (b *Business) Translate(word string) []models.TranslationPairs {
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
	translations := make([]models.TranslationPairs, 0)

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
	}

	return translations
}

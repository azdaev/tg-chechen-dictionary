package tools

import (
	"bytes"
	"chetoru/models"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
)

func Translate(word string) []models.TranslationResponse {
	values := map[string]string{
		"word": word,
	}

	data := new(bytes.Buffer)
	formWriter := multipart.NewWriter(data)
	for key, value := range values {
		_ = formWriter.WriteField(key, value)
	}
	formWriter.Close()

	req, err := http.NewRequest("POST", "https://ps95.ru/dikdosham/dosh.php", data)
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

	response := make(map[string][]models.TranslationResponse)
	json.NewDecoder(resp.Body).Decode(&response)
	translations := make([]models.TranslationResponse, 0)

	for key := range response {
		translation := response[key][0]
		translation.Original = EscapeUnclosedTags(translation.Original)
		translation.Translate = EscapeUnclosedTags(translation.Translate)
		translations = append(translations, translation)
	}

	return translations
}

func Clean(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	output := re.ReplaceAllString(text, "")
	strings.ReplaceAll(output, "<>", ";")
	strings.ReplaceAll(output, "<br />", " ")
	strings.ReplaceAll(output, "\n", " ")
	return output
}

func EscapeUnclosedTags(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	matches := re.FindAllString(text, -1)
	count := 0
	for _, match := range matches {
		if strings.HasPrefix(match, "</") {
			count--
		} else {
			count++
		}
	}
	if count != 0 {
		return Clean(text) //TODO: optimize - one function for check. if true clean
	}
	return text
}

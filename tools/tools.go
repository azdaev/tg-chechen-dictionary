package tools

import (
	"bytes"
	"chetoru/internal/models"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
)

func Translate(word string) []entities.TranslationResponse {
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

	response := make(map[string][]entities.TranslationResponse)
	json.NewDecoder(resp.Body).Decode(&response)
	translations := make([]entities.TranslationResponse, 0)

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

func StatsMessageText(newMonthlyUsers int, monthlyActiveUsers int, dailyActiveUsersInMonth []int) string {
	messageText := fmt.Sprintf(`
<b>Статистика</b>

Новых пользователей за месяц: %d

Активных пользователей за месяц: %d

Уникальных пользователей на протяжении месяца:

`, newMonthlyUsers, monthlyActiveUsers)

	for i, dau := range dailyActiveUsersInMonth {
		day := i + 1
		messageText += fmt.Sprintf("%d - %d\n", day, dau)
	}

	return messageText
}

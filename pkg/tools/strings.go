package tools

import (
	"regexp"
	"strings"
)

func Clean(text string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	output := re.ReplaceAllString(text, "")
	output = strings.ReplaceAll(output, "<>", ";")
	output = strings.ReplaceAll(output, "<br />", " ")
	output = strings.ReplaceAll(output, "\n", " ")
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

package models

type TranslationResponse struct {
	Data struct {
		Find struct {
			Success        bool   `json:"success"`
			SerializedData string `json:"serializedData"`
			ErrorMessage   string `json:"errorMessage"`
		} `json:"find"`
	} `json:"data"`
}

type TranslationPairs struct {
	Original  string `json:"word"`
	Translate string `json:"translate"`
}

type ActivityType int8

const (
	ActivityTypeText   ActivityType = 1
	ActivityTypeInline ActivityType = 2
)

type DailyActivity struct {
	ActiveUsers int
	Calls       int
}

// GraphQL specific types
type Entry struct {
	EntryID      string        `json:"EntryId"`
	Content      string        `json:"Content"`
	Translations []Translation `json:"Translations"`
	SubEntries   []Entry       `json:"SubEntries"`
	Header       string        `json:"Header"`
}

type Translation struct {
	TranslationID string `json:"TranslationId"`
	Content       string `json:"Content"`
	LanguageCode  string `json:"LanguageCode"`
	Notes         string `json:"Notes"`
}

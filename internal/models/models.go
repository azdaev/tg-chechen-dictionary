package models

// TranslationResponse models the dosham.app GraphQL API (https://api.dosham.app/gql).
// The `find` query returns a flat list of entries directly.
type TranslationResponse struct {
	Data struct {
		Find []Entry `json:"find"`
	} `json:"data"`
}

type TranslationPairs struct {
	Original        string `json:"word"`
	Translate       string `json:"translate"`
	FormattedAI     string `json:"formatted_ai,omitempty"`
	FormattedChosen string `json:"formatted_chosen,omitempty"`
}

// TranslationResult содержит переводы и отформатированный текст для кэширования
type TranslationResult struct {
	Pairs     []TranslationPairs `json:"pairs"`
	Formatted string             `json:"formatted"`
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

// MissingWord is a word users searched for that had no translation.
type MissingWord struct {
	CleanWord      string
	RawWord        string
	SearchCount    int
	LastSearchedAt string
}

// RandomWord is a randomly picked dictionary pair oriented as Chechen → Russian,
// used by the /random discovery feature.
type RandomWord struct {
	Chechen string
	Russian string
}

// QuizQuestion is a multiple-choice question for the /quiz feature: a Chechen
// word and several Russian answer options, one of which (CorrectIdx) is right.
type QuizQuestion struct {
	Chechen    string
	Options    []string
	CorrectIdx int
}

// GraphQL specific types (dosham.app /gql schema)
type Entry struct {
	EntryID      string        `json:"entryId"`
	Content      string        `json:"content"`
	Type         string        `json:"type"`
	Details      string        `json:"details"` // JSON grammar metadata (part of speech, case, etc.)
	Translations []Translation `json:"translations"`
}

type Translation struct {
	TranslationID string `json:"translationId"`
	Content       string `json:"content"`
	LanguageCode  string `json:"languageCode"` // ISO codes: "ce" (Chechen), "ru" (Russian)
	Notes         string `json:"notes"`
}

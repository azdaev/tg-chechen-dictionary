package entities

type TranslationResponse struct {
	Original  string `json:"word"`
	Translate string `json:"translate"`
}

type ActivityType int8

const (
	ActivityTypeText   ActivityType = 1
	ActivityTypeInline ActivityType = 2
)

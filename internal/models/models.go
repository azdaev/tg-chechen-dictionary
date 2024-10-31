package models

type TranslationResponse struct {
	Data []struct {
		DictName string `json:"dictName"`
		Words    []struct {
			Id        string `json:"id"`
			Word      string `json:"word"`
			Translate string `json:"translate"`
		} `json:"words"`
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

package models

type TranslationResponse struct {
	Original  string `json:"word"`
	Translate string `json:"translate"`
}

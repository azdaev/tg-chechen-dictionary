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
	// Rate is dosham's own entry weight, used to order results within a
	// relevance bucket. Locally stored pairs have no rate, so ordering must
	// never depend on it alone.
	Rate int `json:"rate,omitempty"`
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

// WordGrammar is lightweight grammatical info about a Chechen headword, sourced
// from the dosham API's `details` (morphology) and `entryForms` (inflected
// forms). Only fields safe to show without the undocumented integer legend are
// kept: the part of speech (inferred from which `details` keys are present) and
// the declension/conjugation paradigm.
type WordGrammar struct {
	Headword string   // the Chechen word
	POS      string   // human-readable part of speech, or "" if uncertain
	Forms    []string // inflected forms (entryForms), unlabeled
	Idioms   []Idiom  // set phrases/collocations (relatedEntries) with a translation
}

// Idiom is a Chechen set phrase or collocation paired with its Russian meaning,
// sourced from the dosham API's relatedEntries.
type Idiom struct {
	Chechen string
	Russian string
}

// QuizQuestion is a multiple-choice question for the /quiz feature: a prompt
// word and several answer options, one of which (CorrectIdx) is right.
// Reversed asks for the Chechen word of a Russian prompt — production recall,
// the harder and more valuable direction for learners.
type QuizQuestion struct {
	Prompt     string
	Options    []string
	CorrectIdx int
	Reversed   bool
}

// QuizScorer is one row of the /top quiz leaderboard. Streak is the current
// run of consecutive practice days (0 when lapsed).
type QuizScorer struct {
	UserID    int64
	Username  string
	FirstName string
	Correct   int
	Total     int
	Streak    int
}

// GraphQL specific types (dosham.app /gql schema)
type Entry struct {
	EntryID      string        `json:"entryId"`
	Content      string        `json:"content"`
	Type         string        `json:"type"`
	Rate         int           `json:"rate"`
	Details      string        `json:"details"` // JSON grammar metadata (part of speech, case, etc.)
	Translations []Translation `json:"translations"`
}

type Translation struct {
	TranslationID string `json:"translationId"`
	Content       string `json:"content"`
	LanguageCode  string `json:"languageCode"` // ISO codes: "ce" (Chechen), "ru" (Russian)
	Notes         string `json:"notes"`
}

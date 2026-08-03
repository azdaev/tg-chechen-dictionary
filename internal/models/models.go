package models

// TranslationResponse models the dosham.app GraphQL API (https://api.dosham.app/gql).
// The `find` query returns a flat list of entries directly.
type TranslationResponse struct {
	Data struct {
		Find []Entry `json:"find"`
	} `json:"data"`
}

type TranslationPairs struct {
	Original  string `json:"word"`
	Translate string `json:"translate"`
	// OriginalLang and TranslateLang are "CHE", "RUS", or "" when unknown. The
	// card bolds whichever side is Chechen and orders examples by it, and no
	// spelling rule can stand in — plenty of Chechen words carry no palochka.
	OriginalLang    string `json:"original_lang,omitempty"`
	TranslateLang   string `json:"translate_lang,omitempty"`
	FormattedAI     string `json:"formatted_ai,omitempty"`
	FormattedChosen string `json:"formatted_chosen,omitempty"`
	// Rate identifies which of dosham's source dictionaries this pair came
	// from, not how good it is. Measured over 1097 live entries it takes three
	// values: 10000 is the academic Chechen–Russian dictionary (senses already
	// atomized one per translation row, stress marks, homonyms numbered), 16 a
	// compact modern one, 100 both the Russian–Chechen dictionary — whose whole
	// article arrives as a single Chechen-language blob — and a software
	// localization glossary. Every tilde and every "1)" in the corpus lives in
	// exactly one of those, so the corpus decides how a pair must be read.
	Rate int `json:"rate,omitempty"`
	// EntryType is dosham's "WORD" or "TEXT": a headword versus a collocation.
	// TEXT entries are the dictionary's own usage examples, already split into
	// the two languages, which is why the card never has to mine them out of an
	// article body.
	EntryType string `json:"entry_type,omitempty"`
	// Subtype is the part of speech. Its values map one-to-one onto the key
	// sets of the `details` JSON (1 verb, 2 noun, 3 adverb, 4 adjective,
	// 6 pronoun), so the label is readable without the undocumented integer
	// legend that keeps noun class off the card.
	Subtype int `json:"subtype,omitempty"`
	// EntryIndex numbers homonyms: цӀа¹ the noun (дом, комната, семья) and цӀа²
	// the adverb (домой) are different words and must not merge into one list.
	EntryIndex int `json:"entry_index,omitempty"`
	// Notes is dosham's usage note on the entry — most often the plural ending
	// ("мн. -аш").
	Notes string `json:"notes,omitempty"`
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
	EntryID string `json:"entryId"`
	Content string `json:"content"`
	Type    string `json:"type"`
	// Subtype is the part of speech; EntryIndex numbers homonyms. Both are
	// plain integers dosham fills in, and both survive into TranslationPairs —
	// see the field docs there for why the card needs them.
	Subtype      int           `json:"subtype"`
	EntryIndex   int           `json:"entryIndex"`
	Notes        string        `json:"notes"`
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

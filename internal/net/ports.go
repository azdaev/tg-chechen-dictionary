// What the Telegram layer needs from everything below it.
package net

import (
	"chetoru/internal/ai"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"context"
	"time"
)

type AI interface {
	SpellCheck(ctx context.Context, text string) (*ai.SpellCheckResult, error)
}

type Business interface {
	Translate(word string) ([]models.TranslationPairs, error)
	TranslateResolved(word string) ([]models.TranslationPairs, string, error)
	SuggestTranslations(word string) []models.TranslationPairs
	SetAIFormatting(enabled bool)
	AIFormattingEnabled() bool
	RandomWordFromAPI(ctx context.Context) (*models.RandomWord, error)
	GenerateQuiz(ctx context.Context) (*models.QuizQuestion, error)
	GrammarFor(ctx context.Context, word string) (*models.WordGrammar, error)
	TranslationCacheStats() (hits, misses int64)
	RecheckTranslation(word string) bool
}

// Repository is the persistence boundary the handlers depend on. It is composed
// from per-domain sub-interfaces so each storage concern can be read (and, in
// tests, mocked) in isolation. The concrete *repository.Repository satisfies the
// whole set; the split is purely for documentation and testability.
type Repository interface {
	UserStore
	StatsStore
	DictionaryStore
	MissingWordStore
	SpellcheckStore
	SubscriptionStore
	QuizStore
	WordOfDayStore
}

// UserStore tracks users, their activity, block state, and donation prompts.
type UserStore interface {
	StoreUser(ctx context.Context, userID int, username string) error
	RecordUserActivity(ctx context.Context, userID int64, username string, activityType models.ActivityType) error
	CountUserActivity(ctx context.Context, userID int64) (int, error)
	ListUserIDs(ctx context.Context) ([]int64, error)
	MarkUserBlocked(ctx context.Context, userID int64, reason string) error
	ShouldSendDonationMessage(ctx context.Context, userID int) (bool, error)
	StoreDonationMessage(ctx context.Context, userID int) error
	WasInlineHinted(ctx context.Context, userID int64) (bool, error)
	MarkInlineHinted(ctx context.Context, userID int64) error
}

// StatsStore answers the aggregate questions behind /stats.
type StatsStore interface {
	CountNewMonthlyUsers(ctx context.Context, month int, year int) (int, error)
	DailyActiveUsersInMonth(ctx context.Context, month int, year int, days int) ([]models.DailyActivity, error)
	MonthlyActiveUsers(ctx context.Context, month int, year int) (int, error)
}

// DictionaryStore holds the user-contributed translation pairs and their
// moderation/formatting state.
type DictionaryStore interface {
	ListPendingTranslationPairs(ctx context.Context, limit int) ([]repository.TranslationPair, error)
	ListPendingTranslationPairsByWord(ctx context.Context, cleanWord string, limit int) ([]repository.TranslationPair, error)
	SetTranslationPairFormattingChoice(ctx context.Context, id int64, choice string) error
	FindTranslationPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	FindStrictlyApprovedPairs(ctx context.Context, cleanWord string, limit int) ([]models.TranslationPairs, error)
	GetPairCleanWords(ctx context.Context, pairID int64) ([]string, error)
	RandomApprovedPair(ctx context.Context) (*models.RandomWord, error)
	CountDictionaryPairs(ctx context.Context) (total int, approved int, err error)
}

// MissingWordStore records lookups that found no translation, so maintainers
// know what coverage to add next.
type MissingWordStore interface {
	RecordMissingWord(ctx context.Context, cleanWord, rawWord string) error
	ResolveMissingWord(ctx context.Context, cleanWord string) error
	TopMissingWords(ctx context.Context, limit int) ([]models.MissingWord, error)
	CountMissingWords(ctx context.Context) (int, error)
}

// SpellcheckStore persists spellcheck feedback and per-user monthly usage (for
// the free-tier quota).
type SpellcheckStore interface {
	StoreSpellcheckFeedback(ctx context.Context, userID int64, originalText, correctedText, feedback string) error
	GetSpellcheckUsage(ctx context.Context, userID int64, month, year int) (int, error)
	IncrementSpellcheckUsage(ctx context.Context, userID int64, month, year int) error
}

// SubscriptionStore tracks paid subscriptions purchased via Telegram Payments.
type SubscriptionStore interface {
	HasActiveSubscription(ctx context.Context, userID int64) (bool, error)
	CreateSubscription(ctx context.Context, userID int64, expiresAt time.Time, telegramPaymentID string) error
}

// QuizStore records /quiz answers and the leaderboard behind /top.
type QuizStore interface {
	RecordQuizAnswer(ctx context.Context, userID int64, username, firstName string, correct bool) error
	GetQuizScore(ctx context.Context, userID int64) (correct, total, streak int, err error)
	TopQuizScorers(ctx context.Context, limit int) ([]models.QuizScorer, error)
	GetQuizRank(ctx context.Context, userID int64) (int, error)
	CountQuizStats(ctx context.Context) (players, totalAnswers, correctAnswers int, err error)
	ListLapsingStreaks(ctx context.Context, lastAnswerDate string) ([]models.QuizScorer, error)
	CountActiveStreaks(ctx context.Context) (int, error)
}

// WordOfDayStore manages opt-in subscriptions for the daily "Word of the Day".
type WordOfDayStore interface {
	SetWordOfDaySubscription(ctx context.Context, userID int64, subscribed bool) error
	IsWordOfDaySubscribed(ctx context.Context, userID int64) (bool, error)
	ListWordOfDaySubscribers(ctx context.Context) ([]int64, error)
	CountWordOfDaySubscribers(ctx context.Context) (int, error)
	WasWordOfDayNudged(ctx context.Context, userID int64) (bool, error)
	MarkWordOfDayNudged(ctx context.Context, userID int64) error
	SetChatWordOfDaySubscription(ctx context.Context, chatID int64, subscribed bool) error
	IsChatWordOfDaySubscribed(ctx context.Context, chatID int64) (bool, error)
	ListWordOfDayChatIDs(ctx context.Context) ([]int64, error)
	CountWordOfDayChats(ctx context.Context) (int, error)
}

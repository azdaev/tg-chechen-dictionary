package cache

import (
	"chetoru/internal/ai"
	"chetoru/internal/models"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// ErrMiss is returned by the Get* methods when a key is absent. Callers use it
// to tell a normal cache miss apart from a real backend failure (Redis down,
// corrupt entry), which should be logged rather than silently swallowed.
var ErrMiss = errors.New("cache: miss")

const (
	translationTTL = 30 * 24 * time.Hour
	// negativeTTL keeps "no results" answers briefly: dead-end queries are the
	// most expensive path (ё-variant fallbacks plus suggestion retries), but
	// new words do get added to dosham, so misses must expire quickly.
	negativeTTL = 24 * time.Hour
	// spellcheckTTL is shorter than translationTTL: the checker prompt evolves,
	// and stale verdicts should pick up improvements within a week.
	spellcheckTTL = 7 * 24 * time.Hour
)

type Cache struct {
	client *redis.Client
}

func NewCache(addr, password string) *Cache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})

	return &Cache{
		client: client,
	}
}

func (c *Cache) Get(ctx context.Context, key string) ([]models.TranslationPairs, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}

	var translations []models.TranslationPairs
	err = json.Unmarshal([]byte(val), &translations)
	if err != nil {
		return nil, err
	}

	return translations, nil
}

func (c *Cache) Set(ctx context.Context, key string, translations []models.TranslationPairs) error {
	data, err := json.Marshal(translations)
	if err != nil {
		return err
	}

	ttl := translationTTL
	if len(translations) == 0 {
		ttl = negativeTTL
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

// wordOfDayKey records the date of the last word-of-day broadcast so a
// restart can tell a missed send from one that already happened. The TTL only
// needs to outlive the catch-up window.
const wordOfDayKey = "wotd_last_sent"

func (c *Cache) GetWordOfDayLastSent(ctx context.Context) (string, error) {
	val, err := c.client.Get(ctx, wordOfDayKey).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (c *Cache) SetWordOfDayLastSent(ctx context.Context, date string) error {
	return c.client.Set(ctx, wordOfDayKey, date, 48*time.Hour).Err()
}

// streakReminderKey records the date of the last streak-reminder sweep, same
// contract as wordOfDayKey.
const streakReminderKey = "streak_reminder_last_sent"

func (c *Cache) GetStreakReminderLastSent(ctx context.Context) (string, error) {
	val, err := c.client.Get(ctx, streakReminderKey).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (c *Cache) SetStreakReminderLastSent(ctx context.Context, date string) error {
	return c.client.Set(ctx, streakReminderKey, date, 48*time.Hour).Err()
}

const (
	wotdRecentKey   = "wotd_recent"
	wotdRecentLimit = 30
)

// RecentWordsOfDay returns the last broadcast word-of-day headwords, newest
// first, so the picker can avoid repeating a word subscribers just saw.
func (c *Cache) RecentWordsOfDay(ctx context.Context) ([]string, error) {
	return c.client.LRange(ctx, wotdRecentKey, 0, -1).Result()
}

func (c *Cache) RememberWordOfDay(ctx context.Context, word string) error {
	pipe := c.client.TxPipeline()
	pipe.LPush(ctx, wotdRecentKey, word)
	pipe.LTrim(ctx, wotdRecentKey, 0, wotdRecentLimit-1)
	_, err := pipe.Exec(ctx)
	return err
}

// SetQuizPoll remembers the correct option for a sent quiz poll so the answer
// can be graded when a poll_answer update arrives. Short-lived — polls are
// answered within minutes.
func (c *Cache) SetQuizPoll(ctx context.Context, pollID string, correctOption int) error {
	return c.client.Set(ctx, "quizpoll_"+pollID, correctOption, 24*time.Hour).Err()
}

// GetQuizPoll returns the stored correct option for a quiz poll, or an error if
// it is unknown/expired.
func (c *Cache) GetQuizPoll(ctx context.Context, pollID string) (int, error) {
	return c.client.Get(ctx, "quizpoll_"+pollID).Int()
}

// grammarCacheEntry wraps a grammar lookup so a "no grammar" answer can be
// cached too: most lookups have no analyzed grammar, and negative caching spares
// the bot from re-running 1–2 live dosham queries for them on every repeat.
type grammarCacheEntry struct {
	Found   bool                `json:"found"`
	Grammar *models.WordGrammar `json:"grammar,omitempty"`
}

// GetGrammar returns the cached grammar for a word. A nil result with a nil
// error is a cached "no grammar" answer; ErrMiss means nothing is cached.
func (c *Cache) GetGrammar(ctx context.Context, key string) (*models.WordGrammar, error) {
	val, err := c.client.Get(ctx, "grammar_"+key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}

	var entry grammarCacheEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, err
	}
	return entry.Grammar, nil
}

// SetGrammar caches a grammar lookup, including a nil ("no grammar") result.
func (c *Cache) SetGrammar(ctx context.Context, key string, g *models.WordGrammar) error {
	data, err := json.Marshal(grammarCacheEntry{Found: g != nil, Grammar: g})
	if err != nil {
		return err
	}
	return c.client.Set(ctx, "grammar_"+key, data, translationTTL).Err()
}

// spellcheckKey hashes the checked text: spellcheck inputs are whole sentences,
// and raw multi-line keys are awkward in Redis.
func spellcheckKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "spellcheck_" + hex.EncodeToString(sum[:])
}

func (c *Cache) GetSpellcheck(ctx context.Context, text string) (*ai.SpellCheckResult, error) {
	val, err := c.client.Get(ctx, spellcheckKey(text)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}

	var result ai.SpellCheckResult
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Cache) SetSpellcheck(ctx context.Context, text string, result *ai.SpellCheckResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, spellcheckKey(text), data, spellcheckTTL).Err()
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

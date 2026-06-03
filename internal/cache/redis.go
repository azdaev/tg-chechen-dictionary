package cache

import (
	"chetoru/internal/models"
	"context"
	"encoding/json"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// ErrMiss is returned by the Get* methods when a key is absent. Callers use it
// to tell a normal cache miss apart from a real backend failure (Redis down,
// corrupt entry), which should be logged rather than silently swallowed.
var ErrMiss = errors.New("cache: miss")

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

	return c.client.Set(ctx, key, data, 24*30*time.Hour).Err()
}

func (c *Cache) GetTranslationResult(ctx context.Context, key string) (*models.TranslationResult, error) {
	val, err := c.client.Get(ctx, "formatted_"+key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}

	var result models.TranslationResult
	err = json.Unmarshal([]byte(val), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Cache) SetTranslationResult(ctx context.Context, key string, result *models.TranslationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, "formatted_"+key, data, 24*30*time.Hour).Err()
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

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

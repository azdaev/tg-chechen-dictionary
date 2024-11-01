package cache

import (
	"chetoru/internal/models"
	"context"
	"encoding/json"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func NewCache(addr string) *Cache {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	return &Cache{
		client: client,
	}
}

func (c *Cache) Get(ctx context.Context, key string) ([]models.TranslationPairs, error) {
	val, err := c.client.Get(ctx, key).Result()
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

	return c.client.Set(ctx, key, data, 24*time.Hour).Err()
}

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

// GetTranslationResult получает кэшированный результат с отформатированным текстом
func (c *Cache) GetTranslationResult(ctx context.Context, key string) (*models.TranslationResult, error) {
	val, err := c.client.Get(ctx, "formatted_"+key).Result()
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

// SetTranslationResult сохраняет результат с отформатированным текстом
func (c *Cache) SetTranslationResult(ctx context.Context, key string, result *models.TranslationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, "formatted_"+key, data, 24*30*time.Hour).Err()
}

// Delete удаляет ключ из кэша
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

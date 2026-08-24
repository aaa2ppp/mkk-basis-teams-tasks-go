package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aaa2ppp/teams-tasks/internal/model"

	"github.com/redis/go-redis/v9"
)

type cache struct {
	redis *redis.Client
	ttl   time.Duration
}

func NewCache(rdb *redis.Client, ttl time.Duration) *cache {
	return &cache{redis: rdb, ttl: ttl}
}

func (c cache) Get(ctx context.Context, key string, val any) error {
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return model.ErrNotFound
		}
		return err
	}
	if err := json.Unmarshal(data, val); err != nil {
		c.redis.Del(ctx, key)
		return fmt.Errorf("received corrupted data from redis: %w", err)
	}
	return nil
}

func (c cache) Put(ctx context.Context, key string, val any) error {
	data, _ := json.Marshal(val)
	return c.redis.Set(ctx, key, data, c.ttl).Err()
}

func (c cache) Invalidate(ctx context.Context, pattern string) error {
	iter := c.redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return c.redis.Unlink(ctx, keys...).Err()
}

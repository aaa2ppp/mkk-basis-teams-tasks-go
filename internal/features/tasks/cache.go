package tasks

import (
	"context"
	"encoding/json"
	"errors"
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
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	return c.redis.Set(ctx, key, data, c.ttl).Err()
}

func (c cache) Invalidate(ctx context.Context, pattern string) error {
	if pattern == "" || pattern == "*" {
		return errors.New("refusing to invalidate all keys")
	}

	const batchSize = 100
	var cursor uint64

	for {
		keys, nextCursor, err := c.redis.Scan(ctx, cursor, pattern, batchSize).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.redis.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

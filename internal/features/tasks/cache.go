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
	redis      *redis.Client
	ttlSeconds int64
}

var _ Cache = &cache{}

func NewCache(rdb *redis.Client, ttl time.Duration) *cache {
	return &cache{redis: rdb, ttlSeconds: int64(ttl / time.Second)}
}

func (c cache) Get(ctx context.Context, key, field string, val any) error {
	data, err := c.redis.HGet(ctx, key, field).Bytes()
	if err != nil {
		if err == redis.Nil {
			return model.ErrNotFound
		}
		return err
	}

	if err := json.Unmarshal(data, val); err != nil {
		c.redis.HDel(ctx, key, field)
		return fmt.Errorf("received corrupted data from redis: %w", err)
	}

	return nil
}

func (c cache) Put(ctx context.Context, key string, field string, val any) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}

	script := `
		redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
		redis.call('EXPIRE', KEYS[1], ARGV[3])
		return 1
	`

	if err := c.redis.Do(ctx, "EVAL", script, 1, key, field, data, c.ttlSeconds).Err(); err != nil {
		return err
	}

	return nil
}

func (c cache) Del(ctx context.Context, key string) error {
	return c.redis.Del(ctx, key).Err()
}

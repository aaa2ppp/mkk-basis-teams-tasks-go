package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/redis/go-redis/v9"
)

type cache struct {
	redis *redis.Client
	ttl   time.Duration
}

var _ Cache = &cache{}

func NewCache(rdb *redis.Client, ttl time.Duration) *cache {
	return &cache{redis: rdb, ttl: ttl}
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
	const op = "tasks.cache.Put"

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}

	script := `
		local created = redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
		local ok, expire_result = pcall(redis.call, 'EXPIRE', KEYS[1], ARGV[3])

		if not ok or expire_result == 0 then
			if created == 1 then
				redis.call('DEL', KEYS[1])
				error('new record but expire failed, rolled back')
			else
				return 2
			end
		end

		return 1
	`

	result, err := c.redis.Eval(ctx, script, []string{key}, field, data, c.ttl.Seconds()).Int()
	if err != nil {
		return fmt.Errorf("eval: %w", err)
	}

	switch result {
	case 1:
		// success
	case 2:
		logger := logging.GetLogger(ctx).With("op", op)
		logger.Warn("data saved but expire failed for existing field", "key", key, "field", field)
	default:
		return fmt.Errorf("unexpected result code %d", result)
	}

	return nil
}

func (c cache) Del(ctx context.Context, key string) error {
	return c.redis.Del(ctx, key).Err()
}

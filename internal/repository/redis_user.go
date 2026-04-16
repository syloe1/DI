package repository

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisUserCache 基于Redis的UserCache实现
type RedisUserCache struct {
	Client *redis.Client
}

func (r *RedisUserCache) Get(ctx context.Context, key string) (string, error) {
	return r.Client.Get(ctx, key).Result()
}

func (r *RedisUserCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.Client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisUserCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return r.Client.SetNX(ctx, key, value, ttl).Result()
}

func (r *RedisUserCache) Del(ctx context.Context, keys ...string) error {
	return r.Client.Del(ctx, keys...).Err()
}
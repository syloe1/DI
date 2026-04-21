package dao

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisUserCache 鍩轰簬Redis鐨刄serCache瀹炵幇
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

func (r *RedisUserCache) Incr(ctx context.Context, key string) (int64, error) {
	return r.Client.Incr(ctx, key).Result()
}

func (r *RedisUserCache) HSet(ctx context.Context, key string, values map[string]interface{}) error {
	return r.Client.HSet(ctx, key, values).Err()
}

func (r *RedisUserCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.Client.HGetAll(ctx, key).Result()
}

func (r *RedisUserCache) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return r.Client.ZIncrBy(ctx, key, increment, member).Result()
}

func (r *RedisUserCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.Client.ZRevRange(ctx, key, start, stop).Result()
}

func (r *RedisUserCache) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZSetMemberScore, error) {
	values, err := r.Client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}

	result := make([]ZSetMemberScore, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			continue
		}
		result = append(result, ZSetMemberScore{
			Member: member,
			Score:  value.Score,
		})
	}
	return result, nil
}

func (r *RedisUserCache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.Client.SAdd(ctx, key, members...).Err()
}

func (r *RedisUserCache) SRem(ctx context.Context, key string, members ...interface{}) error {
	return r.Client.SRem(ctx, key, members...).Err()
}

func (r *RedisUserCache) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.Client.SIsMember(ctx, key, member).Result()
}

func (r *RedisUserCache) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.Client.SMembers(ctx, key).Result()
}

func (r *RedisUserCache) Del(ctx context.Context, keys ...string) error {
	return r.Client.Del(ctx, keys...).Err()
}

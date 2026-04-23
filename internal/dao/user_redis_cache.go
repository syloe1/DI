package dao

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisUserCache 基于 Redis 实现的用户缓存操作类
// 实现了 UserCache 接口，负责处理所有用户相关的缓存逻辑
type RedisUserCache struct {
	Client *redis.Client // Redis 客户端连接
}

// Get 根据 key 获取缓存中的字符串数据
func (r *RedisUserCache) Get(ctx context.Context, key string) (string, error) {
	return r.Client.Get(ctx, key).Result()
}

// Set 存储字符串到缓存，并设置过期时间
func (r *RedisUserCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.Client.Set(ctx, key, value, ttl).Err()
}

// SetNX 原子性设置缓存（仅当 key 不存在时才设置），常用于分布式锁
func (r *RedisUserCache) SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	return r.Client.SetNX(ctx, key, value, ttl).Result()
}

// Incr 对 key 对应的数字进行 +1 操作（可用于计数、限流）
func (r *RedisUserCache) Incr(ctx context.Context, key string) (int64, error) {
	return r.Client.Incr(ctx, key).Result()
}

// HSet 存储哈希结构数据（适合存储用户信息、对象结构）
func (r *RedisUserCache) HSet(ctx context.Context, key string, values map[string]interface{}) error {
	return r.Client.HSet(ctx, key, values).Err()
}

// HGetAll 获取整个哈希结构的所有字段和值
func (r *RedisUserCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.Client.HGetAll(ctx, key).Result()
}

// ZIncrBy 对有序集合中的成员分数增加增量（可用于排行榜）
func (r *RedisUserCache) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return r.Client.ZIncrBy(ctx, key, increment, member).Result()
}

// ZRevRange 按分数从高到低获取有序集合的成员（分页）
func (r *RedisUserCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.Client.ZRevRange(ctx, key, start, stop).Result()
}

// ZRevRangeWithScores 获取有序集合成员及对应的分数（带分值）
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

// SAdd 向集合中添加成员（用于在线用户、去重场景）
func (r *RedisUserCache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return r.Client.SAdd(ctx, key, members...).Err()
}

// SRem 从集合中移除成员
func (r *RedisUserCache) SRem(ctx context.Context, key string, members ...interface{}) error {
	return r.Client.SRem(ctx, key, members...).Err()
}

// SIsMember 判断某个成员是否在集合中（用于判断用户是否在线）
func (r *RedisUserCache) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.Client.SIsMember(ctx, key, member).Result()
}

// SMembers 获取集合中所有成员（如获取所有在线用户ID）
func (r *RedisUserCache) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.Client.SMembers(ctx, key).Result()
}

// Del 删除一个或多个缓存key
func (r *RedisUserCache) Del(ctx context.Context, keys ...string) error {
	return r.Client.Del(ctx, keys...).Err()
}

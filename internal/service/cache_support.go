package service

import (
	"context"
	"math/rand"
	"time"

	"go-admin/internal/dao"
)

const (
	cacheNullValue  = "null"
	defaultCacheTTL = 5 * time.Minute
	cacheNullTTL    = 1 * time.Minute
	cacheLockTTL    = 5 * time.Second
	cacheRetryTimes = 5
	cacheRetryDelay = 50 * time.Millisecond
	maxCacheJitter  = 120
)

// 缓存抖动TTL
func jitterTTL(base time.Duration) time.Duration {
	return base + time.Duration(rand.Intn(maxCacheJitter))*time.Second
}

// 缓存重试机制
func spinWaitCache(cache dao.UserCache, ctx context.Context, key string) (string, bool) {
	for i := 0; i < cacheRetryTimes; i++ {
		time.Sleep(cacheRetryDelay)
		val, err := cache.Get(ctx, key)
		if err == nil {
			return val, true
		}
	}
	return "", false
}

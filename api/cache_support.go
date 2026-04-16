package api

import (
	"context"
	"math/rand"
	"time"

	"go-admin/internal/repository"
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

func jitterTTL(base time.Duration) time.Duration {
	return base + time.Duration(rand.Intn(maxCacheJitter))*time.Second
}

func spinWaitCache(cache repository.UserCache, ctx context.Context, key string) (string, bool) {
	for i := 0; i < cacheRetryTimes; i++ {
		time.Sleep(cacheRetryDelay)
		val, err := cache.Get(ctx, key)
		if err == nil {
			return val, true
		}
	}
	return "", false
}
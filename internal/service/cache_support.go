package service

import (
	"context"
	"math/rand"
	"time"

	"go-admin/internal/dao"
)

const (
	cacheNullValue  = "null"                // 缓存空值标记（解决缓存穿透）
	defaultCacheTTL = 5 * time.Minute       // 正常数据缓存过期时间
	cacheNullTTL    = 1 * time.Minute       // 空值缓存过期时间（比正常短）
	cacheLockTTL    = 5 * time.Second       // 分布式锁过期时间（防死锁）
	cacheRetryTimes = 5                     // 缓存读取最大重试次数
	cacheRetryDelay = 50 * time.Millisecond // 每次重试间隔
	maxCacheJitter  = 120                   // TTL 随机抖动最大秒数
)

// 缓存抖动TTL
//type Duration int64
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

package middleware

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type tokenBucket struct {
	tokens     float64
	capacity   float64
	rate       float64   //每秒放多少令牌
	lastRefill time.Time //最后放令牌时间
}

type inMemoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

// P1-3 修复：限制器持有后台清理 goroutine，
// 定期删除超过 expireAfter 未访问的 bucket，避免 map 只增不减导致内存泄漏。
const (
	limiterCleanupInterval = 10 * time.Minute
	limiterExpireAfter     = 10 * time.Minute
)

func newInMemoryLimiter() *inMemoryLimiter {
	l := &inMemoryLimiter{
		buckets: make(map[string]*tokenBucket),
	}
	go l.cleanupLoop()
	return l
}

var defaultLimiter = newInMemoryLimiter()

func (l *inMemoryLimiter) cleanupLoop() {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, b := range l.buckets {
			if now.Sub(b.lastRefill) > limiterExpireAfter {
				delete(l.buckets, key)
			}
		}
		l.mu.Unlock()
	}
}

func RateLimit(name string, rate float64, capacity int, keyFunc func(*gin.Context) (string, bool)) gin.HandlerFunc {
	if rate <= 0 {
		rate = 1
	}
	if capacity <= 0 {
		capacity = 1
	}

	return func(c *gin.Context) {
		key, ok := keyFunc(c)
		if !ok || key == "" {
			core.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		allowed, waitSeconds := defaultLimiter.allow(name+":"+key, rate, capacity)
		if !allowed {
			if waitSeconds > 0 {
				c.Header("Retry-After", fmt.Sprintf("%.0f", math.Ceil(waitSeconds)))
			}
			core.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}

func ClientIPKey(c *gin.Context) (string, bool) {
	ip := c.ClientIP()
	return ip, ip != ""
}

func UserIDKey(c *gin.Context) (string, bool) {
	userID := c.GetUint("userID")
	if userID == 0 {
		return "", false
	}
	return fmt.Sprintf("user:%d", userID), true
}

func (l *inMemoryLimiter) allow(key string, rate float64, capacity int) (bool, float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	bucket, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &tokenBucket{
			tokens:     float64(capacity - 1),
			capacity:   float64(capacity),
			rate:       rate,
			lastRefill: now,
		}
		return true, 0
	}

	elapsed := now.Sub(bucket.lastRefill).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(bucket.capacity, bucket.tokens+elapsed*bucket.rate)
		bucket.lastRefill = now
	}

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, 0
	}

	deficit := 1 - bucket.tokens
	return false, deficit / bucket.rate
}

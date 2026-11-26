package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"di/internal/telemetry"
	"di/internal/transport"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
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

// rateLimitRedis 分布式限流后端；为 nil 时仅用本地内存限流。
var rateLimitRedis *redis.Client

// SetRateLimitRedis 注入 Redis 用于分布式限流（多实例共享额度）。
// Redis 故障时 RateLimit 自动降级到本地单机限流。
func SetRateLimitRedis(client *redis.Client) {
	rateLimitRedis = client
}

// 分布式令牌桶（Lua 原子）：ARGV[1]=每秒令牌数，ARGV[2]=桶容量，ARGV[3]=now(ms)，ARGV[4]=ttl(秒)
var rateLimitScript = redis.NewScript(`
	local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens'))
	local last = tonumber(redis.call('HGET', KEYS[1], 'last'))
	local now = tonumber(ARGV[3])
	local rate = tonumber(ARGV[1])
	local capacity = tonumber(ARGV[2])
	if tokens == nil then
		tokens = capacity
		last = now
	end
	local elapsed = (now - last) / 1000.0
	tokens = math.min(capacity, tokens + elapsed * rate)
	local allowed = 0
	if tokens >= 1 then
		tokens = tokens - 1
		allowed = 1
	end
	redis.call('HSET', KEYS[1], 'tokens', tokens, 'last', now)
	redis.call('EXPIRE', KEYS[1], tonumber(ARGV[4]))
	return allowed
`)

func redisAllow(ctx context.Context, key string, rate float64, capacity int) (bool, error) {
	ttl := int(math.Ceil(float64(capacity)/rate)) + 1
	if ttl < 2 {
		ttl = 2
	}
	res, err := rateLimitScript.Run(ctx, rateLimitRedis, []string{"rl:" + key}, rate, capacity, time.Now().UnixMilli(), ttl).Result()
	if err != nil {
		return false, err
	}
	return res.(int64) == 1, nil
}

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
			transport.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		limitKey := name + ":" + key
		allowed := false
		waitSeconds := 0.0

		if rateLimitRedis != nil {
			if ok, err := redisAllow(c.Request.Context(), limitKey, rate, capacity); err == nil {
				allowed = ok
			} else {
				// Redis 故障：降级到本地单机限流（可用性优先，仅本机额度）
				telemetry.Z().Warnf("redis rate limit failed, fallback to local limiter: %v", err)
				allowed, waitSeconds = defaultLimiter.allow(limitKey, rate, capacity)
			}
		} else {
			allowed, waitSeconds = defaultLimiter.allow(limitKey, rate, capacity)
		}

		if !allowed {
			if waitSeconds > 0 {
				c.Header("Retry-After", fmt.Sprintf("%.0f", math.Ceil(waitSeconds)))
			}
			transport.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
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

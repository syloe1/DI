package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"go-admin/internal/dao"

	"github.com/go-redis/redis/v8"
)

// JWT 主动失效（logout / 改密后旧 token 立即不可用）。
//
// 方案：Redis 存两类 key，均自带 TTL，不依赖定时清理：
//  1. auth:jwt:blacklist:<sha256(token)> = "1"     —— 登出时写入，TTL 到该 token 过期为止；
//  2. auth:jwt:invalid_before:<userID>   = unix毫秒 —— 改密/强制下线时写入，
//     签发时间早于该时间戳的 token 全部失效（覆盖同一用户的其他设备）。
//
// 为什么用毫秒：JWT 标准 iat 只有秒级精度，同一秒内「先签发 → 再改密」的 token
// 用秒级比较会漏掉；新 token 自带 iat_ms 毫秒声明，比较精度到毫秒。
//
// 为什么用 sha256 而不是原始 token：Redis 泄露时无法直接拿来重放；同时 key 定长。
const (
	jwtBlacklistKeyPrefix     = "auth:jwt:blacklist:"
	jwtInvalidBeforeKeyPrefix = "auth:jwt:invalid_before:"
)

// parseInvalidBefore 解析 user 级失效时间戳（毫秒）；兼容旧版本秒级存储。
func parseInvalidBefore(val string) (int64, bool) {
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	// 小于 1e12 视为秒级（2026 年秒级约 1.7e9，毫秒级约 1.7e12）
	if n < 1_000_000_000_000 {
		n *= 1000
	}
	return n, true
}

// TokenRevocationStore 实现 HTTP 中间件 / WebSocket 握手需要的主动失效检查。
// 放在 service 层，middleware 只依赖接口，避免反向依赖。
type TokenRevocationStore struct {
	cache dao.UserCache
	ttl   time.Duration // user 级失效标记的 TTL（应 >= JWT 有效期）
}

func NewTokenRevocationStore(cache dao.UserCache, jwtTTL time.Duration) *TokenRevocationStore {
	if jwtTTL <= 0 {
		jwtTTL = 24 * time.Hour
	}
	return &TokenRevocationStore{cache: cache, ttl: jwtTTL + time.Hour}
}

func tokenFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Revoke 把单个 token 加入黑名单，TTL 到 expiresAt 为止（之后 JWT 自然过期即可）。
func (s *TokenRevocationStore) Revoke(ctx context.Context, token string, userID uint, expiresAt time.Time) error {
	if s == nil || s.cache == nil || token == "" {
		return nil
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // 已经过期的 token 不需要再写黑名单
	}
	return s.cache.Set(ctx, jwtBlacklistKeyPrefix+tokenFingerprint(token), "1", ttl)
}

// IsRevoked 判断 token 是否已主动失效。
// 返回 error 时调用方按 fail-open 处理（Redis 抖动不阻断所有请求），但会记录告警日志。
func (s *TokenRevocationStore) IsRevoked(ctx context.Context, token string, userID uint, issuedAt time.Time) (bool, error) {
	if s == nil || s.cache == nil {
		return false, nil
	}

	if token != "" {
		val, err := s.cache.Get(ctx, jwtBlacklistKeyPrefix+tokenFingerprint(token))
		if err == nil {
			return val != "", nil
		}
		if !errors.Is(err, redis.Nil) {
			return false, err
		}
	}

	if userID > 0 {
		key := jwtInvalidBeforeKeyPrefix + strconv.FormatUint(uint64(userID), 10)
		val, err := s.cache.Get(ctx, key)
		if err == nil {
			if invalidBefore, ok := parseInvalidBefore(val); ok && !issuedAt.IsZero() && issuedAt.UnixMilli() < invalidBefore {
				return true, nil
			}
			return false, nil
		}
		if !errors.Is(err, redis.Nil) {
			return false, err
		}
	}

	return false, nil
}

// InvalidateUserBefore 让 userID 在 at 之前签发的所有 token 失效（改密 / 强制下线）。
func (s *TokenRevocationStore) InvalidateUserBefore(ctx context.Context, userID uint, at time.Time) error {
	if s == nil || s.cache == nil || userID == 0 {
		return nil
	}
	key := jwtInvalidBeforeKeyPrefix + strconv.FormatUint(uint64(userID), 10)
	return s.cache.Set(ctx, key, strconv.FormatInt(at.UnixMilli(), 10), s.ttl)
}

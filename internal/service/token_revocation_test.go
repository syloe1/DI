package service

import (
	"context"
	"testing"
	"time"

	"di/internal/dao"

	"github.com/go-redis/redis/v8"
)

// fakeTokenCache 只实现 token 失效逻辑用到的 Get/Set；其余方法通过嵌入接口占位。
type fakeTokenCache struct {
	dao.UserCache
	values map[string]string
	ttls   map[string]time.Duration
}

func newFakeTokenCache() *fakeTokenCache {
	return &fakeTokenCache{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (f *fakeTokenCache) Get(_ context.Context, key string) (string, error) {
	if v, ok := f.values[key]; ok {
		return v, nil
	}
	return "", redis.Nil
}

func (f *fakeTokenCache) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	f.values[key] = value
	f.ttls[key] = ttl
	return nil
}

func TestTokenRevocationStoreRevoke(t *testing.T) {
	ctx := context.Background()
	cache := newFakeTokenCache()
	store := NewTokenRevocationStore(cache, time.Hour)

	expiresAt := time.Now().Add(30 * time.Minute)
	if err := store.Revoke(ctx, "token-a", 7, expiresAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	revoked, err := store.IsRevoked(ctx, "token-a", 7, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if !revoked {
		t.Fatal("expected token-a to be revoked")
	}

	// 指纹相同但 token 不同 → 不受影响
	revoked, err = store.IsRevoked(ctx, "token-b", 7, time.Now())
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Fatal("expected token-b not to be revoked")
	}
}

func TestTokenRevocationStoreSkipsExpiredToken(t *testing.T) {
	cache := newFakeTokenCache()
	store := NewTokenRevocationStore(cache, time.Hour)

	if err := store.Revoke(context.Background(), "expired", 1, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("Revoke expired: %v", err)
	}
	if len(cache.values) != 0 {
		t.Fatalf("expired token should not be stored, got %d keys", len(cache.values))
	}
}

func TestTokenRevocationStoreInvalidateUserBefore(t *testing.T) {
	ctx := context.Background()
	cache := newFakeTokenCache()
	store := NewTokenRevocationStore(cache, 2*time.Hour)

	invalidAt := time.Now()
	if err := store.InvalidateUserBefore(ctx, 42, invalidAt); err != nil {
		t.Fatalf("InvalidateUserBefore: %v", err)
	}

	oldToken, err := store.IsRevoked(ctx, "old", 42, invalidAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("IsRevoked old: %v", err)
	}
	if !oldToken {
		t.Fatal("token issued before invalid_before should be revoked")
	}

	newToken, err := store.IsRevoked(ctx, "new", 42, invalidAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("IsRevoked new: %v", err)
	}
	if newToken {
		t.Fatal("token issued after invalid_before should stay valid")
	}
}

func TestTokenRevocationStoreSameSecondPrecision(t *testing.T) {
	ctx := context.Background()
	cache := newFakeTokenCache()
	store := NewTokenRevocationStore(cache, 2*time.Hour)

	// 同一秒内：先签发的 token 必须被改密失效，后签发的（新登录）必须有效
	base := time.Unix(1_700_000_000, 0)
	invalidAt := base.Add(900 * time.Millisecond)
	if err := store.InvalidateUserBefore(ctx, 7, invalidAt); err != nil {
		t.Fatalf("InvalidateUserBefore: %v", err)
	}

	before, err := store.IsRevoked(ctx, "same-second-before", 7, base.Add(100*time.Millisecond))
	if err != nil {
		t.Fatalf("IsRevoked before: %v", err)
	}
	if !before {
		t.Fatal("token issued 100ms before invalidation (same second) should be revoked")
	}

	after, err := store.IsRevoked(ctx, "same-second-after", 7, base.Add(950*time.Millisecond))
	if err != nil {
		t.Fatalf("IsRevoked after: %v", err)
	}
	if after {
		t.Fatal("token issued 50ms after invalidation should stay valid")
	}
}

func TestTokenRevocationStoreLegacySecondsValue(t *testing.T) {
	ctx := context.Background()
	cache := newFakeTokenCache()
	store := NewTokenRevocationStore(cache, 2*time.Hour)

	// 旧版本写入的是秒级时间戳：读取时应自动按秒级兼容
	key := jwtInvalidBeforeKeyPrefix + "42"
	cache.values[key] = "1700000000"

	oldToken, err := store.IsRevoked(ctx, "legacy-old", 42, time.Unix(1_699_999_999, 0))
	if err != nil {
		t.Fatalf("IsRevoked legacy old: %v", err)
	}
	if !oldToken {
		t.Fatal("token older than legacy seconds marker should be revoked")
	}

	newToken, err := store.IsRevoked(ctx, "legacy-new", 42, time.Unix(1_700_000_001, 0))
	if err != nil {
		t.Fatalf("IsRevoked legacy new: %v", err)
	}
	if newToken {
		t.Fatal("token newer than legacy seconds marker should stay valid")
	}
}

func TestVerifyInviteCode(t *testing.T) {
	svc := &UserService{authOpts: AuthOptions{RegisterInviteCode: "s3cret"}}

	if err := svc.verifyInviteCode("s3cret"); err != nil {
		t.Fatalf("correct invite code should pass, got %v", err)
	}
	if err := svc.verifyInviteCode(""); err == nil {
		t.Fatal("missing invite code should be rejected")
	}
	if err := svc.verifyInviteCode("wrong"); err == nil {
		t.Fatal("wrong invite code should be rejected")
	}

	// 未配置邀请码时放开注册
	openSvc := &UserService{}
	if err := openSvc.verifyInviteCode(""); err != nil {
		t.Fatalf("empty config should allow register, got %v", err)
	}
}

package service

import (
	"context"
	"fmt"
	"time"

	"di/internal/domain/model"
)

// 在线状态统一由 PresenceService 管理（Redis online:uid + TTL + 心跳续期），
// 不再使用独立的 user:online Set（S2-11 唯一来源）。

const userProfileCacheTTL = 30 * time.Minute

func userProfileCacheKey(userID uint) string {
	return fmt.Sprintf("user:profile:%d", userID)
}

func buildSafeUserProfile(user *model.User) map[string]interface{} {
	if user == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	}
}

func (s *UserService) cacheUserProfile(ctx context.Context, user *model.User) {
	if user == nil || user.ID == 0 {
		return
	}
	key := userProfileCacheKey(user.ID)
	_ = s.cache.HSet(ctx, key, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
	// 兜底 TTL：正常生命周期靠显式失效（deleteUserProfileCache），TTL 防止孤儿 key 累积
	_ = s.cache.Expire(ctx, key, userProfileCacheTTL)
}

func (s *UserService) deleteUserProfileCache(ctx context.Context, userID uint) {
	if userID == 0 {
		return
	}
	_ = s.cache.Del(ctx, userProfileCacheKey(userID), fmt.Sprintf("user:%d", userID))
}

func (s *UserService) getCachedUserProfile(ctx context.Context, userID uint) (map[string]interface{}, bool) {
	values, err := s.cache.HGetAll(ctx, userProfileCacheKey(userID))
	if err != nil || len(values) == 0 {
		return nil, false
	}

	profile := map[string]interface{}{
		"id":       userID,
		"username": values["username"],
		"role":     values["role"],
	}
	if createdAt := values["created_at"]; createdAt != "" {
		profile["created_at"] = createdAt
	}
	return profile, true
}

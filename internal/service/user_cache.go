package service

import (
	"context"
	"fmt"
	"time"

	"go-admin/internal/domain/model"
)

// 在线状态统一由 PresenceService 管理（Redis online:uid + TTL + 心跳续期），
// 不再使用独立的 user:online Set（S2-11 唯一来源）。

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
	_ = s.cache.HSet(ctx, userProfileCacheKey(user.ID), map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
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

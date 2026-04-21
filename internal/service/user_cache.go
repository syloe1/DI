package service

import (
	"fmt"
	"time"

	"go-admin/internal/domain/model"
)

const OnlineUsersKey = "user:online"

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

func (s *UserService) cacheUserProfile(user *model.User) {
	if user == nil || user.ID == 0 {
		return
	}
	_ = s.cache.HSet(s.ctx, userProfileCacheKey(user.ID), map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
}

func (s *UserService) deleteUserProfileCache(userID uint) {
	if userID == 0 {
		return
	}
	_ = s.cache.Del(s.ctx, userProfileCacheKey(userID), fmt.Sprintf("user:%d", userID))
}

func (s *UserService) getCachedUserProfile(userID uint) (map[string]interface{}, bool) {
	values, err := s.cache.HGetAll(s.ctx, userProfileCacheKey(userID))
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
	if online, err := s.cache.SIsMember(s.ctx, OnlineUsersKey, userID); err == nil {
		profile["is_online"] = online
	}
	return profile, true
}

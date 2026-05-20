package gateway

import (
	"context"

	"go-admin/internal/service"
)

type RedisPresenceAdapter struct {
	presence *service.PresenceService
}

func NewRedisPresenceAdapter(presence *service.PresenceService) *RedisPresenceAdapter {
	return &RedisPresenceAdapter{presence: presence}
}

func (a *RedisPresenceAdapter) RegisterOnline(ctx context.Context, uid string) error {
	if a == nil || a.presence == nil {
		return nil
	}
	return a.presence.RegisterOnlineUID(ctx, uid)
}

func (a *RedisPresenceAdapter) UnregisterOnline(ctx context.Context, uid string) error {
	if a == nil || a.presence == nil {
		return nil
	}
	return a.presence.UnregisterOnlineUID(ctx, uid)
}

func (a *RedisPresenceAdapter) RefreshOnline(ctx context.Context, uid string) error {
	if a == nil || a.presence == nil {
		return nil
	}
	return a.presence.RefreshOnlineUID(ctx, uid)
}

func (a *RedisPresenceAdapter) Shutdown(ctx context.Context) error {
	if a == nil || a.presence == nil {
		return nil
	}
	return a.presence.Shutdown(ctx)
}

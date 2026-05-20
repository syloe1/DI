package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

/*
用户上线 → 记录在线状态
用户下线 → 删除在线状态
用户保持连接 → 定时刷新过期时间
服务停机 → 自动清理该服务上的所有用户
*/
const presenceTTL = 90 * time.Second //90s过期

// ======================
// 1. 用户上线 Lua
// ======================
var registerOnlineScript = redis.NewScript(`
	-- KEYS[1] = online:uid
	-- KEYS[2] = route:uid
	-- KEYS[3] = instance:users:instanceID
	-- ARGV[1] = ttl 秒数
	-- ARGV[2] = instanceID
	-- ARGV[3] = uid

	redis.call("SET", KEYS[1], "1", "EX", ARGV[1])
	redis.call("SET", KEYS[2], ARGV[2], "EX", ARGV[1])
	redis.call("SADD", KEYS[3], ARGV[3])
	redis.call("EXPIRE", KEYS[3], ARGV[1])
	return 1
`)

// ======================
// 2. 用户下线 Lua
// ======================
var unregisterOnlineScript = redis.NewScript(`
	-- KEYS[1] = online:uid
	-- KEYS[2] = route:uid
	-- KEYS[3] = instance:users:instanceID
	-- ARGV[1] = uid

	redis.call("DEL", KEYS[1])
	redis.call("DEL", KEYS[2])
	redis.call("SREM", KEYS[3], ARGV[1])
	return 1
`)

// ======================
// 3. 心跳续期 Lua（全新）
// ======================
var refreshOnlineScript = redis.NewScript(`
	-- KEYS[1] = online:uid
	-- KEYS[2] = route:uid
	-- KEYS[3] = instance:users:instanceID
	-- ARGV[1] = ttl 秒数

	redis.call("EXPIRE", KEYS[1], ARGV[1])
	redis.call("EXPIRE", KEYS[2], ARGV[1])
	redis.call("EXPIRE", KEYS[3], ARGV[1])
	return 1
`)

// ======================
// 4. 服务停机清理 Lua（全新）
// ======================
var shutdownScript = redis.NewScript(`
	-- KEYS[1] = instance:xxx:users
	-- 功能：获取该实例所有用户，批量删除在线状态 + 删除集合

	local users = redis.call("SMEMBERS", KEYS[1])
	for i, uid in ipairs(users) do
		redis.call("DEL", "online:" .. uid)
		redis.call("DEL", "route:" .. uid)
	end
	redis.call("DEL", KEYS[1])
	return 1
`)

/*
用户连上来 → 执行这个脚本
其他服务想知道：
用户在线吗？查 online:uid
用户连在哪台服务器？查 route:uid
这台服务器有谁在线？查 instance:users:xxx
超时自动下线，不用手动删
*/
type PresenceService struct {
	client     *redis.Client
	instanceID string //服务器唯一ID
	ttl        time.Duration
}

func NewPresenceService(client *redis.Client, instanceID string) *PresenceService {
	return &PresenceService{
		client:     client,
		instanceID: instanceID,
		ttl:        presenceTTL,
	}
}

func (s *PresenceService) InstanceID() string {
	return s.instanceID
}

func (s *PresenceService) RegisterOnline(ctx context.Context, userID uint) error {
	return s.RegisterOnlineUID(ctx, strconv.FormatUint(uint64(userID), 10))
}

// ======================
// 上线 - Lua 版本
// ======================
func (s *PresenceService) RegisterOnlineUID(ctx context.Context, uid string) error {
	if s == nil || s.client == nil || uid == "" {
		return nil
	}

	keyOnline := onlineKey(uid)
	keyRoute := routeKey(uid)
	keyInstanceUsers := instanceUsersKey(s.instanceID)
	ttlSeconds := int(s.ttl.Seconds())

	_, err := registerOnlineScript.Run(ctx,
		s.client,
		[]string{keyOnline, keyRoute, keyInstanceUsers},
		ttlSeconds,
		s.instanceID,
		uid,
	).Result()

	return err
}

func (s *PresenceService) UnregisterOnline(ctx context.Context, userID uint) error {
	return s.UnregisterOnlineUID(ctx, strconv.FormatUint(uint64(userID), 10))
}

// ======================
// 下线 - Lua 版本
// ======================
func (s *PresenceService) UnregisterOnlineUID(ctx context.Context, uid string) error {
	if s == nil || s.client == nil || uid == "" {
		return nil
	}

	keyOnline := onlineKey(uid)
	keyRoute := routeKey(uid)
	keyInstanceUsers := instanceUsersKey(s.instanceID)

	_, err := unregisterOnlineScript.Run(ctx,
		s.client,
		[]string{keyOnline, keyRoute, keyInstanceUsers},
		uid,
	).Result()

	return err
}

func (s *PresenceService) RefreshOnline(ctx context.Context, userID uint) error {
	return s.RefreshOnlineUID(ctx, strconv.FormatUint(uint64(userID), 10))
}

// ======================
// 心跳续期 - Lua 版本
// ======================
func (s *PresenceService) RefreshOnlineUID(ctx context.Context, uid string) error {
	if s == nil || s.client == nil || uid == "" {
		return nil
	}

	keyOnline := onlineKey(uid)
	keyRoute := routeKey(uid)
	keyInstanceUsers := instanceUsersKey(s.instanceID)
	ttlSeconds := int(s.ttl.Seconds())

	_, err := refreshOnlineScript.Run(ctx,
		s.client,
		[]string{keyOnline, keyRoute, keyInstanceUsers},
		ttlSeconds,
	).Result()

	return err
}

// ======================
// 服务停机 - Lua 版本（超级强！）
// ======================
func (s *PresenceService) Shutdown(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}

	keyInstanceUsers := instanceUsersKey(s.instanceID)

	// 🔥 一次性原子完成：获取用户 + 批量删除 + 清理实例
	_, err := shutdownScript.Run(ctx,
		s.client,
		[]string{keyInstanceUsers},
	).Result()

	return err
}

// ======================
// KEY 工具函数
// ======================
func onlineKey(uid string) string {
	return fmt.Sprintf("online:%s", uid)
}

func routeKey(uid string) string {
	return fmt.Sprintf("route:%s", uid)
}

func instanceUsersKey(instanceID string) string {
	return fmt.Sprintf("instance:%s:users", instanceID)
}

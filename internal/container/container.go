package container

import (
	"context"
	"fmt"
	"os"

	"di/internal/config"
	"di/internal/handler"
	"di/internal/infra"
	"di/internal/service"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Container 由 wire 生成的 InitializeContainer 组装，持有所有应用级依赖与生命周期资源。
type Container struct {
	Config    *config.App
	Logger    *zap.SugaredLogger
	DB        *gorm.DB
	Redis     *redis.Client
	JWTSecret []byte

	UserService *service.UserService
	UserHandler *handler.UserHandler
	// TokenRevocation JWT 主动失效（logout/改密后旧 token 立即 401）
	TokenRevocation *service.TokenRevocationStore

	WSService      *service.WSService
	WSHandler      *handler.WSHandler
	MessageService *service.MessageService
	MessageHandler *handler.MessageHandler
	GroupHandler   *handler.GroupHandler
	GroupService   *service.GroupService

	// 生命周期管理（优雅关闭 / MQ 重连）
	Rabbit         *infra.RabbitMQ
	Presence       *service.PresenceService
	Outbox         *service.OutboxService
	Consumer       *service.GroupMessageConsumer
	cancelConsumer context.CancelFunc
}

// Shutdown 按「启动逆序」清理资源：
// WS 连接 → Redis 在线状态 → outbox/consumer → RabbitMQ。
func (c *Container) Shutdown(ctx context.Context) {
	if c == nil {
		return
	}
	// 1. 关闭 WebSocket 连接（unregisterClient 会触发 Presence 清理）
	if c.WSService != nil && c.WSService.Hub() != nil {
		c.WSService.Hub().Shutdown(ctx)
	}
	// 2. 兜底：Lua 原子清理本实例 Redis 在线状态
	if c.Presence != nil {
		_ = c.Presence.Shutdown(ctx)
	}
	// 3. 停 outbox worker 与 MQ consumer（ctx 取消）
	if c.cancelConsumer != nil {
		c.cancelConsumer()
	}
	// 4. 关闭 RabbitMQ（触发重连循环退出）
	if c.Rabbit != nil {
		_ = c.Rabbit.Close()
	}
}

func resolveInstanceID() string {
	if val := os.Getenv("INSTANCE_ID"); val != "" {
		return val
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	// 主机名 + 进程 pid
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

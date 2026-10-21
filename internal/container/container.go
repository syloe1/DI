package container

import (
	"context"
	"fmt"
	"os"
	"time"

	"go-admin/config"
	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/handler"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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
	Rabbit         *core.RabbitMQ
	Presence       *service.PresenceService
	Outbox         *service.OutboxService
	Consumer       *service.GroupMessageConsumer
	cancelConsumer context.CancelFunc
}

func NewContainer(cfg *config.App, db *gorm.DB, redisClient *redis.Client, appLogger *zap.SugaredLogger) *Container {
	ctx := context.Background()
	jwtSecret := []byte(cfg.GetJwtConfig().Secret)

	userDB := &dao.GormUserDB{DB: db}
	userCache := &dao.RedisUserCache{Client: redisClient}
	messageRepo := dao.NewGormMessageRepository(db)
	outboxRepo := dao.NewGormOutboxRepository(db)
	jwtCfg := &dao.DefaultJWTConfig{
		Secret:      jwtSecret,
		ExpireHours: time.Duration(cfg.GetJwtConfig().ExpireHours) * time.Hour,
	}
	groupRepo := dao.NewGormGroupRepository(db)
	instanceID := resolveInstanceID()
	instanceQueue := fmt.Sprintf("%s.%s", cfg.RabbitMQ.Queue, instanceID)
	presenceService := service.NewPresenceService(redisClient, instanceID)
	groupCacheService := service.NewGroupCacheService(redisClient, groupRepo)
	// JWT 主动失效：Redis 黑名单 + 用户级 invalid_before（ttl 覆盖整个 token 生命周期）
	tokenRevocation := service.NewTokenRevocationStore(userCache, time.Duration(cfg.GetJwtConfig().ExpireHours)*time.Hour)
	rabbit, err := core.NewRabbitMQ(cfg.RabbitMQ.URL, cfg.RabbitMQ.Exchange, instanceQueue)
	if err != nil {
		core.Z().Fatalf("connect rabbitmq failed: %v", err)
	}
	groupMessagePublisher := service.NewRabbitGroupMessagePublisher(rabbit, cfg.RabbitMQ.Exchange)
	outboxService := service.NewOutboxService(outboxRepo, groupMessagePublisher, instanceID)
	outboxService.Start(ctx)

	userService := service.NewUserService(userDB, userCache, jwtCfg, jwtSecret, presenceService, tokenRevocation, service.AuthOptions{
		RegisterInviteCode: cfg.GetAuthConfig().RegisterInviteCode,
		DemoEnabled:        cfg.GetDemoConfig().Enabled,
		DemoUsername:       cfg.GetDemoConfig().Username,
	})
	userHandler := handler.NewUserHandler(userService)
	wsService := service.NewWSService(messageRepo, groupRepo, groupMessagePublisher, presenceService, groupCacheService, userCache, ctx, jwtSecret, tokenRevocation)
	// 用户角色/权限变更 → 强制断开其 WS（踢下线），presence 随之清理在线状态
	userService.SetOnUserKicked(func(userID uint) { wsService.KickUser(userID) })
	wsHandler := handler.NewWSHandler(wsService)

	// consumer 使用独立 ctx：优雅关闭和 MQ 重连时取消并重启
	consumerCtx, cancelConsumer := context.WithCancel(ctx)
	groupMessageConsumer := service.NewGroupMessageConsumer(rabbit, instanceQueue, groupRepo, groupCacheService, userCache, wsService.Hub())
	if err := groupMessageConsumer.Start(consumerCtx); err != nil {
		core.Z().Fatalf("start group message consumer failed: %v", err)
	}

	// MQ 断线自动重连；重连成功后取消旧 consumer，在新通道上重启
	go rabbit.RunReconnectLoop(ctx, func() {
		cancelConsumer()
		consumerCtx, cancelConsumer = context.WithCancel(ctx)
		if err := groupMessageConsumer.Start(consumerCtx); err != nil {
			core.Z().Errorf("restart group message consumer failed: %v", err)
		}
	})

	messageService := service.NewMessageService(messageRepo, userCache, func(senderUID uint, readerUID uint, messageIDs []uint) {
		wsService.SendToUser(senderUID, map[string]interface{}{
			"type": "message.read",
			"data": map[string]interface{}{
				"peer_uid":    readerUID,
				"message_ids": messageIDs,
				"read_at":     time.Now(),
			},
		})
	}, func(msg *model.Message) {
		// HTTP 私信落库后实时推给收件人（离线静默，sync 兜底）
		wsService.SendToUser(msg.ToUID, map[string]interface{}{
			"type": "message",
			"data": map[string]interface{}{
				"id":         msg.ID,
				"from_uid":   msg.FromUID,
				"to_uid":     msg.ToUID,
				"content":    msg.Content,
				"created_at": msg.CreatedAt.Format(time.RFC3339),
			},
		})
	})
	messageHandler := handler.NewMessageHandler(messageService)
	groupService := service.NewGroupService(groupRepo, groupCacheService)
	groupHandler := handler.NewGroupHandler(groupService)
	return &Container{
		Config:          cfg,
		Logger:          appLogger,
		DB:              db,
		Redis:           redisClient,
		JWTSecret:       jwtSecret,
		UserService:     userService,
		UserHandler:     userHandler,
		TokenRevocation: tokenRevocation,
		WSService:       wsService,
		WSHandler:       wsHandler,
		MessageService:  messageService,
		MessageHandler:  messageHandler,
		GroupHandler:    groupHandler,
		GroupService:    groupService,
		Rabbit:          rabbit,
		Presence:        presenceService,
		Outbox:          outboxService,
		Consumer:        groupMessageConsumer,
		cancelConsumer:  cancelConsumer,
	}
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
	//主机名 + 进程pID去做
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

package container

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go-admin/config"
	"go-admin/internal/dao"
	"go-admin/internal/handler"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type Container struct {
	Config    *config.App
	Logger    *log.Logger
	DB        *gorm.DB
	Redis     *redis.Client
	JWTSecret []byte

	UserService *service.UserService
	UserHandler *handler.UserHandler
	PostService *service.PostService
	PostHandler *handler.PostHandler

	WSService       *service.WSService
	WSHandler       *handler.WSHandler
	CommentService  *service.CommentService
	CommentHandler  *handler.CommentHandler
	MessageService  *service.MessageService
	MessageHandler  *handler.MessageHandler
	SocialService   *service.SocialService
	SocialHandler   *handler.SocialHandler
	InteractService *service.InteractService
	InteractHandler *handler.InteractHandler
	GroupHandler    *handler.GroupHandler
	GroupService    *service.GroupService

	// 生命周期管理（优雅关闭 / MQ 重连）
	Rabbit         *core.RabbitMQ
	Presence       *service.PresenceService
	Outbox         *service.OutboxService
	Consumer       *service.GroupMessageConsumer
	cancelConsumer context.CancelFunc
}

func NewContainer(cfg *config.App, db *gorm.DB, redisClient *redis.Client, appLogger *log.Logger) *Container {
	ctx := context.Background()
	jwtSecret := []byte(cfg.GetJwtConfig().Secret)

	userDB := &dao.GormUserDB{DB: db}
	userCache := &dao.RedisUserCache{Client: redisClient}
	postRepo := dao.NewGormPostRepository(db)
	commentRepo := dao.NewGormCommentRepository(db)
	interactRepo := dao.NewGormInteractRepository(db)
	socialRepo := dao.NewGormSocialRepository(db)
	messageRepo := dao.NewGormMessageRepository(db)
	outboxRepo := dao.NewGormOutboxRepository(db)
	jwtCfg := &dao.DefaultJWTConfig{Secret: jwtSecret}
	groupRepo := dao.NewGormGroupRepository(db)
	instanceID := resolveInstanceID()
	instanceQueue := fmt.Sprintf("%s.%s", cfg.RabbitMQ.Queue, instanceID)
	presenceService := service.NewPresenceService(redisClient, instanceID)
	groupCacheService := service.NewGroupCacheService(redisClient, groupRepo)
	rabbit, err := core.NewRabbitMQ(cfg.RabbitMQ.URL, cfg.RabbitMQ.Exchange, instanceQueue)
	if err != nil {
		log.Fatalf("connect rabbitmq failed: %v", err)
	}
	groupMessagePublisher := service.NewRabbitGroupMessagePublisher(rabbit, cfg.RabbitMQ.Exchange)
	outboxService := service.NewOutboxService(outboxRepo, groupMessagePublisher, instanceID)
	outboxService.Start(ctx)

	userService := service.NewUserService(userDB, userCache, jwtCfg, jwtSecret, presenceService)
	userHandler := handler.NewUserHandler(userService)
	postService := service.NewPostService(postRepo, userCache)
	postHandler := handler.NewPostHandler(postService)
	wsService := service.NewWSService(messageRepo, groupRepo, groupMessagePublisher, presenceService, groupCacheService, userCache, ctx, jwtSecret)
	// 用户角色/权限变更 → 强制断开其 WS（踢下线），presence 随之清理在线状态
	userService.SetOnUserKicked(func(userID uint) { wsService.KickUser(userID) })
	wsHandler := handler.NewWSHandler(wsService)

	// consumer 使用独立 ctx：优雅关闭和 MQ 重连时取消并重启
	consumerCtx, cancelConsumer := context.WithCancel(ctx)
	groupMessageConsumer := service.NewGroupMessageConsumer(rabbit, instanceQueue, groupRepo, groupCacheService, userCache, wsService.Hub())
	if err := groupMessageConsumer.Start(consumerCtx); err != nil {
		log.Fatalf("start group message consumer failed: %v", err)
	}

	// MQ 断线自动重连；重连成功后取消旧 consumer，在新通道上重启
	go rabbit.RunReconnectLoop(ctx, func() {
		cancelConsumer()
		consumerCtx, cancelConsumer = context.WithCancel(ctx)
		if err := groupMessageConsumer.Start(consumerCtx); err != nil {
			log.Printf("restart group message consumer failed: %v", err)
		}
	})

	commentService := service.NewCommentService(commentRepo)
	commentHandler := handler.NewCommentHandler(commentService)
	// 已读回执：sender 发给 reader 的消息被 reader 已读后，给 sender 推 WS 事件 "message.read"
	messageService := service.NewMessageService(messageRepo, userCache, func(senderUID uint, readerUID uint, messageIDs []uint) {
		wsService.SendToUser(senderUID, map[string]interface{}{
			"type": "message.read",
			"data": map[string]interface{}{
				"peer_uid":    readerUID,
				"message_ids": messageIDs,
				"read_at":     time.Now(),
			},
		})
	})
	messageHandler := handler.NewMessageHandler(messageService)
	socialService := service.NewSocialService(socialRepo)
	socialHandler := handler.NewSocialHandler(socialService)
	interactService := service.NewInteractService(interactRepo, userCache)
	interactHandler := handler.NewInteractHandler(interactService)
	groupService := service.NewGroupService(groupRepo)
	groupHandler := handler.NewGroupHandler(groupService)
	return &Container{
		Config:          cfg,
		Logger:          appLogger,
		DB:              db,
		Redis:           redisClient,
		JWTSecret:       jwtSecret,
		UserService:     userService,
		UserHandler:     userHandler,
		PostService:     postService,
		PostHandler:     postHandler,
		WSService:       wsService,
		WSHandler:       wsHandler,
		CommentService:  commentService,
		CommentHandler:  commentHandler,
		MessageService:  messageService,
		MessageHandler:  messageHandler,
		SocialService:   socialService,
		SocialHandler:   socialHandler,
		InteractService: interactService,
		InteractHandler: interactHandler,
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

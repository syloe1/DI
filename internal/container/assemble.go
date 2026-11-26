package container

import (
	"context"
	"fmt"

	"di/internal/config"
	"di/internal/handler"
	"di/internal/infra"
	"di/internal/service"
	"di/internal/telemetry"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// assembleContainer 是 wire 装配链的最后一步：所有依赖已构造完毕，
// 这里完成 setter 注入与生命周期启动，返回完整 Container。
func assembleContainer(
	ctx context.Context,
	cfg *config.App,
	logger *zap.SugaredLogger,
	db *gorm.DB,
	redisClient *redis.Client,
	jwtSecret []byte,
	userService *service.UserService,
	userHandler *handler.UserHandler,
	tokenRevocation *service.TokenRevocationStore,
	wsService *service.WSService,
	wsHandler *handler.WSHandler,
	messageService *service.MessageService,
	messageHandler *handler.MessageHandler,
	groupHandler *handler.GroupHandler,
	groupService *service.GroupService,
	rabbit *infra.RabbitMQ,
	presence *service.PresenceService,
	outbox *service.OutboxService,
	consumer *service.GroupMessageConsumer,
) (*Container, error) {
	// setter 注入：绕开「UserService ↔ WSService / hub ↔ MessageService」的构造期循环。
	userService.SetOnUserKicked(func(userID uint) { wsService.KickUser(userID) })
	wsService.Hub().SetServices(messageService, groupService)

	// 生命周期：outbox worker → consumer → MQ 断线重连。
	outbox.Start(ctx)

	consumerCtx, cancelConsumer := context.WithCancel(ctx)
	if err := consumer.Start(consumerCtx); err != nil {
		cancelConsumer()
		return nil, fmt.Errorf("start group message consumer: %w", err)
	}
	// MQ 断线自动重连；重连成功后取消旧 consumer，在新通道上重启。
	go rabbit.RunReconnectLoop(ctx, func() {
		cancelConsumer()
		consumerCtx, cancelConsumer = context.WithCancel(ctx)
		if err := consumer.Start(consumerCtx); err != nil {
			telemetry.Z().Errorf("restart group message consumer failed: %v", err)
		}
	})

	return &Container{
		Config:          cfg,
		Logger:          logger,
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
		Presence:        presence,
		Outbox:          outbox,
		Consumer:        consumer,
		cancelConsumer:  cancelConsumer,
	}, nil
}

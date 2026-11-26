//go:build wireinject

package container

import (
	"di/internal/config"
	"di/internal/dao"
	"di/internal/handler"
	"di/internal/service"

	"github.com/go-redis/redis/v8"
	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InitializeContainer 由 wire 生成实现：按依赖顺序构造全部服务，并完成 setter/生命周期装配。
func InitializeContainer(cfg *config.App, db *gorm.DB, redisClient *redis.Client, appLogger *zap.SugaredLogger) (*Container, error) {
	wire.Build(
		// 基础提供者（config 派生值 / 结构体包装）
		provideContext,
		provideJWTSecret,
		provideJWTTTL,
		provideInfraConfig,
		provideUserDB,
		provideUserCache,
		provideJWTConfig,
		provideAuthOptions,
		provideRabbitMQ,
		provideGroupMessagePublisher,
		providePresence,
		provideOutboxService,
		provideGroupMessageConsumer,
		provideWSHub,
		provideWSService,
		provideMessageService,

		// 直接使用现有构造器
		dao.NewGormMessageRepository,
		dao.NewGormGroupRepository,
		dao.NewGormOutboxRepository,
		service.NewGroupCacheService,
		service.NewTokenRevocationStore,
		service.NewUserService,
		service.NewGroupService,
		handler.NewUserHandler,
		handler.NewWSHandler,
		handler.NewMessageHandler,
		handler.NewGroupHandler,

		// 接口绑定：具体实现 -> 接口（service 依赖接口，store 提供实现）
		wire.Bind(new(dao.UserDB), new(*dao.GormUserDB)),
		wire.Bind(new(dao.UserCache), new(*dao.RedisUserCache)),
		wire.Bind(new(dao.JWTConfig), new(*dao.DefaultJWTConfig)),
		wire.Bind(new(dao.MessageRepository), new(*dao.GormMessageRepository)),
		wire.Bind(new(dao.GroupRepository), new(*dao.GormGroupRepository)),
		wire.Bind(new(dao.OutboxRepository), new(*dao.GormOutboxRepository)),
		wire.Bind(new(service.GroupMessagePublisher), new(*service.RabbitGroupMessagePublisher)),
		wire.Bind(new(service.OutboxPublisher), new(*service.RabbitGroupMessagePublisher)),

		// 最终装配（setter + 生命周期）
		assembleContainer,
	)
	return nil, nil
}

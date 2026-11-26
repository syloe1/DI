package container

import (
	"context"
	"fmt"
	"time"

	"di/internal/config"
	"di/internal/dao"
	"di/internal/domain/model"
	"di/internal/infra"
	"di/internal/service"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// infraConfig 把散落的字符串配置（instanceID / queue / exchange / url）打包成单一类型，
// 避免 wire 面对多个 string 值无法区分来源。
type infraConfig struct {
	URL        string
	Exchange   string
	Queue      string
	InstanceID string
}

func provideContext() context.Context {
	return context.Background()
}

func provideJWTSecret(cfg *config.App) []byte {
	return []byte(cfg.GetJwtConfig().Secret)
}

func provideJWTTTL(cfg *config.App) time.Duration {
	return time.Duration(cfg.GetJwtConfig().ExpireHours) * time.Hour
}

func provideInfraConfig(cfg *config.App) infraConfig {
	id := resolveInstanceID()
	return infraConfig{
		URL:        cfg.RabbitMQ.URL,
		Exchange:   cfg.RabbitMQ.Exchange,
		Queue:      fmt.Sprintf("%s.%s", cfg.RabbitMQ.Queue, id),
		InstanceID: id,
	}
}

func provideUserDB(db *gorm.DB) *dao.GormUserDB {
	return &dao.GormUserDB{DB: db}
}

func provideUserCache(client *redis.Client) *dao.RedisUserCache {
	return &dao.RedisUserCache{Client: client}
}

func provideJWTConfig(secret []byte, ttl time.Duration) *dao.DefaultJWTConfig {
	return &dao.DefaultJWTConfig{Secret: secret, ExpireHours: ttl}
}

func provideAuthOptions(cfg *config.App) service.AuthOptions {
	return service.AuthOptions{
		RegisterInviteCode: cfg.GetAuthConfig().RegisterInviteCode,
		DemoEnabled:        cfg.GetDemoConfig().Enabled,
		DemoUsername:       cfg.GetDemoConfig().Username,
	}
}

func provideRabbitMQ(cfg infraConfig) (*infra.RabbitMQ, error) {
	return infra.NewRabbitMQ(cfg.URL, cfg.Exchange, cfg.Queue)
}

func provideGroupMessagePublisher(rabbit *infra.RabbitMQ, cfg infraConfig) *service.RabbitGroupMessagePublisher {
	return service.NewRabbitGroupMessagePublisher(rabbit, cfg.Exchange)
}

func providePresence(client *redis.Client, cfg infraConfig) *service.PresenceService {
	return service.NewPresenceService(client, cfg.InstanceID)
}

func provideOutboxService(repo dao.OutboxRepository, publisher service.OutboxPublisher, cfg infraConfig) *service.OutboxService {
	return service.NewOutboxService(repo, publisher, cfg.InstanceID)
}

func provideGroupMessageConsumer(
	rabbit *infra.RabbitMQ,
	cfg infraConfig,
	groupRepo dao.GroupRepository,
	groupCache *service.GroupCacheService,
	cache dao.UserCache,
	hub *service.WSHub,
) *service.GroupMessageConsumer {
	return service.NewGroupMessageConsumer(rabbit, cfg.Queue, groupRepo, groupCache, cache, hub)
}

// provideWSHub 复用 WSService 内部创建的 hub，避免给 consumer 造出第二个 hub。
func provideWSHub(ws *service.WSService) *service.WSHub {
	return ws.Hub()
}

// provideWSService 包装 NewWSService：其 revoker 参数是未导出接口 wsTokenRevoker，
// container 包无法引用它，这里用具体类型 *TokenRevocationStore 传入即可满足。
func provideWSService(
	messageRepo dao.MessageRepository,
	groupRepo dao.GroupRepository,
	publisher service.GroupMessagePublisher,
	presence *service.PresenceService,
	groupCache *service.GroupCacheService,
	cache dao.UserCache,
	ctx context.Context,
	jwtSecret []byte,
	revoker *service.TokenRevocationStore,
) *service.WSService {
	return service.NewWSService(messageRepo, groupRepo, publisher, presence, groupCache, cache, ctx, jwtSecret, revoker)
}

// provideMessageService 包装 NewMessageService 的两个闭包回调（依赖 wsService 实时推送）。
func provideMessageService(
	messageRepo dao.MessageRepository,
	cache dao.UserCache,
	ws *service.WSService,
) *service.MessageService {
	return service.NewMessageService(
		messageRepo,
		cache,
		func(senderUID uint, readerUID uint, messageIDs []uint) {
			ws.SendToUser(senderUID, map[string]interface{}{
				"type": "message.read",
				"data": map[string]interface{}{
					"peer_uid":    readerUID,
					"message_ids": messageIDs,
					"read_at":     time.Now(),
				},
			})
		},
		func(msg *model.Message) {
			// HTTP 私信落库后实时推给收件人（离线静默，sync 兜底）
			ws.SendToUser(msg.ToUID, map[string]interface{}{
				"type": "message",
				"data": map[string]interface{}{
					"id":         msg.ID,
					"from_uid":   msg.FromUID,
					"to_uid":     msg.ToUID,
					"content":    msg.Content,
					"created_at": msg.CreatedAt.Format(time.RFC3339),
				},
			})
		},
	)
}

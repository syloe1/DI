package service

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"

	"github.com/rabbitmq/amqp091-go"
)

const (
	defaultGroupMessageConsumerWorkers = 8   // MQ 消费协程数
	defaultPushShardWorkers            = 16  // 消息推送协程数

	groupMessageProcessedKeyPrefix = "group_message_processed:" // 幂等Key前缀
	groupMessageProcessingTTL      = 5 * time.Minute             // 处理中状态过期时间
	groupMessageProcessedTTL       = 7 * 24 * time.Hour          // 已完成状态过期时间
)

type GroupMessageConsumer struct {
	ch          *amqp091.Channel   // RabbitMQ 信道
	queue       string             // 消费队列名
	groupRepo   dao.GroupRepository// 群数据库DAO
	groupCache  *GroupCacheService // 群成员/在线缓存服务（前面整套分片缓存）
	cache       dao.UserCache      // Redis 缓存客户端（用于幂等去重）
	hub         *WSHub             // WebSocket 连接管理器，负责推送消息
	workers     int                // MQ 消费协程数
	pushWorkers int                // 分片推送协程数
}

func NewGroupMessageConsumer(ch *amqp091.Channel, queue string, groupRepo dao.GroupRepository, groupCache *GroupCacheService, cache dao.UserCache, hub *WSHub) *GroupMessageConsumer {
	return &GroupMessageConsumer{
		ch:          ch,
		queue:       queue,
		groupRepo:   groupRepo,
		groupCache:  groupCache,
		cache:       cache,
		hub:         hub,
		workers:     defaultGroupMessageConsumerWorkers,
		pushWorkers: defaultPushShardWorkers,
	}
}

func (c *GroupMessageConsumer) Start(ctx context.Context) error {
	if c.workers <= 0 {
		c.workers = defaultGroupMessageConsumerWorkers
	}

	if err := c.ch.Qos(c.workers*2, 0, false); err != nil {
		return err
	}
	//订阅队列
	deliveries, err := c.ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	//缓冲
	tasks := make(chan amqp091.Delivery, c.workers*2)
	var wg sync.WaitGroup

	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.workerLoop(ctx, tasks)
		}()
	}
	//消息转发协程
	go func() {
		defer close(tasks)
		defer wg.Wait()

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					//重回队列
					_ = d.Nack(false, true)
					return
				case tasks <- d:
				}
			}
		}
	}()

	return nil
}

func (c *GroupMessageConsumer) workerLoop(ctx context.Context, tasks <-chan amqp091.Delivery) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-tasks:
			if !ok {
				return
			}
			if err := c.handleDelivery(ctx, d); err != nil {
				log.Printf("consume group message event failed: %v", err)
				core.Metrics.ConsumerFailed.Add(1)
				_ = d.Nack(false, false)
				continue
			}
			//手动确认
			_ = d.Ack(false)
			core.Metrics.ConsumerSucceeded.Add(1)
		}
	}
}

func (c *GroupMessageConsumer) handleDelivery(ctx context.Context, d amqp091.Delivery) error {
	var event dto.GroupMessageCreatedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		return err
	}
	if event.Type != "group_message_created" {
		return nil
	}
	dedupeKey := groupMessageDedupeKey(event.MessageID, event.RequestID)
	if dedupeKey == "" {
		return nil
	}
    //幂等机制
	processed, err := c.markProcessing(ctx, dedupeKey)
	if err != nil {
		return err
	}
	//如果正在消费
	if processed {
		return nil
	}

	group, err := c.groupRepo.FindGroupByID(event.GroupID)
	if err != nil {
		_ = c.clearProcessing(ctx, dedupeKey)
		return err
	}
	if group.Status != model.ChatGroupStatusNormal {
		//不再推送消息
		_ = c.markDone(ctx, dedupeKey)
		return nil
	}

	out, err := json.Marshal(dto.WSOutboundMessage{
		Type:      "group_message",
		MessageID: event.MessageID,
		GroupID:   event.GroupID,
		FromUID:   event.FromUID,
		Content:   event.Content,
		Time:      event.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		_ = c.clearProcessing(ctx, dedupeKey)
		return err
	}

	shards, err := c.targetOnlineShards(ctx, event.GroupID)
	if err != nil {
		_ = c.clearProcessing(ctx, dedupeKey)
		return err
	}
	//分片并发推送
	c.pushShards(ctx, shards, out)
	if err := c.markDone(ctx, dedupeKey); err != nil {
		log.Printf("mark consumer event done failed: %v", err)
	}

	return nil
}

func (c *GroupMessageConsumer) markProcessing(ctx context.Context, dedupeKey string) (bool, error) {
	if c.cache == nil || dedupeKey == "" {
		return false, nil
	}

	key := groupMessageProcessedKey(dedupeKey)
	ok, err := c.cache.SetNX(ctx, key, "processing", groupMessageProcessingTTL)
	if err != nil {
		return false, err
	}
	if ok {
		return false, nil
	}
	// 查询当前状态，判断是否已处理 / 处理中
	value, err := c.cache.Get(ctx, key)
	if err != nil {
		return false, err
	}

	return value == "done" || value == "processing", nil
}

func (c *GroupMessageConsumer) markDone(ctx context.Context, dedupeKey string) error {
	if c.cache == nil || dedupeKey == "" {
		return nil
	}

	return c.cache.Set(ctx, groupMessageProcessedKey(dedupeKey), "done", groupMessageProcessedTTL)
}

func (c *GroupMessageConsumer) clearProcessing(ctx context.Context, dedupeKey string) error {
	if c.cache == nil || dedupeKey == "" {
		return nil
	}

	return c.cache.Del(ctx, groupMessageProcessedKey(dedupeKey))
}

func groupMessageDedupeKey(messageID uint, requestID string) string {
	if messageID > 0 {
		return strconv.FormatUint(uint64(messageID), 10)
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	return "req:" + requestID
}

func groupMessageProcessedKey(dedupeKey string) string {
	return groupMessageProcessedKeyPrefix + dedupeKey
}

func (c *GroupMessageConsumer) targetOnlineShards(ctx context.Context, groupID uint) ([][]uint, error) {
	if c.groupCache != nil {
		return c.groupCache.ActiveOnlineMemberIDsByShard(ctx, groupID)
	}

	members, err := c.groupRepo.ListActiveMembers(groupID)
	if err != nil {
		return nil, err
	}
	//单个分片
	shards := make([][]uint, 1)
	localUsers := c.hub.GetOnlineUserSet()
	for _, member := range members {
		if _, ok := localUsers[member.UserID]; ok {
			shards[0] = append(shards[0], member.UserID)
		}
	}
	return shards, nil
}

func (c *GroupMessageConsumer) pushShards(ctx context.Context, shards [][]uint, payload []byte) {
	if c.pushWorkers <= 0 {
		c.pushWorkers = defaultPushShardWorkers
	}

	tasks := make(chan []uint, len(shards))
	var wg sync.WaitGroup

	for i := 0; i < c.pushWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for users := range tasks {
				for _, userID := range users {
					select {
					case <-ctx.Done():
						return
					default:
						c.hub.SendMessageToUser(userID, payload)
					}
				}
			}
		}()
	}

	for _, users := range shards {
		if len(users) == 0 {
			continue
		}
		tasks <- users
	}
	close(tasks)
	wg.Wait()
}

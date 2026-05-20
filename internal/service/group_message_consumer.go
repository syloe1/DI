package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"

	"github.com/rabbitmq/amqp091-go"
)

const (
	defaultGroupMessageConsumerWorkers = 8
	defaultPushShardWorkers            = 16
)

type GroupMessageConsumer struct {
	ch          *amqp091.Channel
	queue       string
	groupRepo   dao.GroupRepository
	groupCache  *GroupCacheService
	hub         *WSHub
	workers     int
	pushWorkers int
}

func NewGroupMessageConsumer(ch *amqp091.Channel, queue string, groupRepo dao.GroupRepository, groupCache *GroupCacheService, hub *WSHub) *GroupMessageConsumer {
	return &GroupMessageConsumer{
		ch:          ch,
		queue:       queue,
		groupRepo:   groupRepo,
		groupCache:  groupCache,
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

	deliveries, err := c.ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	tasks := make(chan amqp091.Delivery, c.workers*2)
	var wg sync.WaitGroup

	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.workerLoop(ctx, tasks)
		}()
	}

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
			_ = d.Ack(false)
			core.Metrics.ConsumerSucceeded.Add(1)
		}
	}
}

// 消费MQ队列里的群消息创建事件 + 推送在线用户
func (c *GroupMessageConsumer) handleDelivery(ctx context.Context, d amqp091.Delivery) error {
	var event dto.GroupMessageCreatedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		return err
	}
	if event.Type != "group_message_created" {
		return nil
	}
	//查询群消息，检测群正常
	group, err := c.groupRepo.FindGroupByID(event.GroupID)
	if err != nil {
		return err
	}
	if group.Status != model.ChatGroupStatusNormal {
		return nil
	}
	//组装消息
	out, err := json.Marshal(dto.WSOutboundMessage{
		Type:      "group_message",
		MessageID: event.MessageID,
		GroupID:   event.GroupID,
		FromUID:   event.FromUID,
		Content:   event.Content,
		Time:      event.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	//获取群内 [活跃 + 在线]的用户切片
	shards, err := c.targetOnlineShards(ctx, event.GroupID)
	if err != nil {
		return err
	}
	c.pushShards(ctx, shards, out)
	return nil
}

func (c *GroupMessageConsumer) targetOnlineShards(ctx context.Context, groupID uint) ([][]uint, error) {
	if c.groupCache != nil {
		return c.groupCache.ActiveOnlineMemberIDsByShard(ctx, groupID)
	}
	//本地计算
	members, err := c.groupRepo.ListActiveMembers(groupID)
	if err != nil {
		return nil, err
	}
	//全部放一个分片
	shards := make([][]uint, 1)
	localUsers := c.hub.GetOnlineUserSet()
	for _, member := range members {
		if _, ok := localUsers[member.UserID]; ok {
			shards[0] = append(shards[0], member.UserID)
		}
	}
	return shards, nil
}

// 把群消息按16分片的在线用户， 用协程池推送给所有人
func (c *GroupMessageConsumer) pushShards(ctx context.Context, shards [][]uint, payload []byte) {
	if c.pushWorkers <= 0 {
		c.pushWorkers = defaultPushShardWorkers
	}

	tasks := make(chan []uint, len(shards))
	var wg sync.WaitGroup
	//16个消费者协程
	for i := 0; i < c.pushWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			//不断从tasks通道按任务
			for users := range tasks {
				//遍历分片的用户
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

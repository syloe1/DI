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

	"github.com/rabbitmq/amqp091-go"
)

const defaultGroupMessageConsumerWorkers = 8

type GroupMessageConsumer struct {
	ch        *amqp091.Channel
	queue     string
	groupRepo dao.GroupRepository
	hub       *WSHub
	workers   int
}

func NewGroupMessageConsumer(ch *amqp091.Channel, queue string, groupRepo dao.GroupRepository, hub *WSHub) *GroupMessageConsumer {
	return &GroupMessageConsumer{
		ch:        ch,
		queue:     queue,
		groupRepo: groupRepo,
		hub:       hub,
		workers:   defaultGroupMessageConsumerWorkers,
	}
}

func (c *GroupMessageConsumer) Start(ctx context.Context) error {
	if c.workers <= 0 {
		c.workers = defaultGroupMessageConsumerWorkers
	}

	if err := c.ch.Qos(c.workers*2, 0, false); err != nil {
		return err
	}
	//监听队列
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
			if err := c.handleDelivery(d); err != nil {
				log.Printf("consume group message event failed: %v", err)
				_ = d.Nack(false, true)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func (c *GroupMessageConsumer) handleDelivery(d amqp091.Delivery) error {
	var event dto.GroupMessageCreatedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		return err
	}
	if event.Type != "group_message_created" {
		return nil
	}

	group, err := c.groupRepo.FindGroupByID(event.GroupID)
	if err != nil {
		return err
	}
	if group.Status != model.ChatGroupStatusNormal {
		return nil
	}

	members, err := c.groupRepo.ListActiveMembers(event.GroupID)
	if err != nil {
		return err
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
		return err
	}

	localUsers := c.hub.GetOnlineUserSet()
	for _, member := range members {
		if _, ok := localUsers[member.UserID]; !ok {
			continue
		}
		c.hub.SendMessageToUser(member.UserID, out)
	}

	return nil
}

package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"

	"github.com/rabbitmq/amqp091-go"
)

type GroupMessageConsumer struct {
	ch        *amqp091.Channel // RabbitMQ 通道
	queue     string
	groupRepo dao.GroupRepository
	hub       *WSHub
}

func NewGroupMessageConsumer(ch *amqp091.Channel, queue string, groupRepo dao.GroupRepository, hub *WSHub) *GroupMessageConsumer {
	return &GroupMessageConsumer{
		ch:        ch,
		queue:     queue,
		groupRepo: groupRepo,
		hub:       hub,
	}
}

// Start () 启动消费者
func (c *GroupMessageConsumer) Start(ctx context.Context) error {
	//监听队列，开始消费
	deliveries, err := c.ch.Consume(
		c.queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}
	//开一个协程，无限循环监听消息
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}

				if err := c.handleDelivery(d); err != nil {
					log.Printf("consume group message event failed: %v", err)
					//处理失败，把消息重新放回队列
					_ = d.Nack(false, true)
					continue
				}

				_ = d.Ack(false)
			}
		}
	}()

	return nil
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

	for _, m := range members {
		c.hub.SendMessageToUser(m.UserID, out)
	}

	return nil
}

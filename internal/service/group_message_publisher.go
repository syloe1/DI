package service

// 把消息发送给mq
import (
	"context"
	"encoding/json"

	"go-admin/internal/dto"
	"go-admin/pkg/core"

	"github.com/rabbitmq/amqp091-go"
)

type GroupMessagePublisher interface {
	PublishGroupMessageCreated(ctx context.Context, event dto.GroupMessageCreatedEvent) error
}

type RabbitGroupMessagePublisher struct {
	rabbit   *core.RabbitMQ
	exchange string
}

func NewRabbitGroupMessagePublisher(rabbit *core.RabbitMQ, exchange string) *RabbitGroupMessagePublisher {
	return &RabbitGroupMessagePublisher{
		rabbit:   rabbit,
		exchange: exchange,
	}
}

func (p *RabbitGroupMessagePublisher) PublishGroupMessageCreated(ctx context.Context, event dto.GroupMessageCreatedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.PublishRaw(ctx, body)
}

func (p *RabbitGroupMessagePublisher) PublishRaw(ctx context.Context, body []byte) error {
	// 每次从 RabbitMQ 现取当前通道：断线重连后自动指向新通道
	return p.rabbit.PublishChan().PublishWithContext(
		ctx,
		p.exchange,
		"",
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Body:         body,
		},
	)
}

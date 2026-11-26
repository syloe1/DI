package service

// 把消息发送给mq
import (
	"context"
	"encoding/json"

	"di/internal/dto"
	"di/internal/infra"
	"github.com/rabbitmq/amqp091-go"
)

type GroupMessagePublisher interface {
	PublishGroupMessageCreated(ctx context.Context, event dto.GroupMessageCreatedEvent) error
}

type RabbitGroupMessagePublisher struct {
	rabbit   *infra.RabbitMQ
	exchange string
}

func NewRabbitGroupMessagePublisher(rabbit *infra.RabbitMQ, exchange string) *RabbitGroupMessagePublisher {
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
	// 走带 publisher confirm 的发布：broker 确认持久化后才算成功。
	// outbox 据此才标记 published（套接字写成功不再等于已投递），失败则走现有重试/失败逻辑。
	return p.rabbit.PublishWithConfirm(
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

package service

import (
	"context"
	"encoding/json"

	"go-admin/internal/dto"

	"github.com/rabbitmq/amqp091-go"
)

type GroupMessagePublisher interface {
	PublishGroupMessageCreated(ctx context.Context, event dto.GroupMessageCreatedEvent) error
}

type RabbitGroupMessagePublisher struct {
	ch       *amqp091.Channel
	exchange string
}

func NewRabbitGroupMessagePublisher(ch *amqp091.Channel, exchange string) *RabbitGroupMessagePublisher {
	return &RabbitGroupMessagePublisher{
		ch:       ch,
		exchange: exchange,
	}
}

func (p *RabbitGroupMessagePublisher) PublishGroupMessageCreated(ctx context.Context, event dto.GroupMessageCreatedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(
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

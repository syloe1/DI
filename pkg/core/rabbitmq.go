package core

import "github.com/rabbitmq/amqp091-go"

type RabbitMQ struct {
	Conn           *amqp091.Connection
	PublishChannel *amqp091.Channel
	ConsumeChannel *amqp091.Channel
}

func NewRabbitMQ(url, exchange, queue string) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}
	//创建发布通道
	publishCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	//创建消费通道
	consumeCh, err := conn.Channel()
	if err != nil {
		_ = publishCh.Close()
		_ = conn.Close()
		return nil, err
	}

	if err := publishCh.ExchangeDeclare(exchange, "fanout", true, false, false, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	if _, err := consumeCh.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	if err := consumeCh.QueueBind(queue, "", exchange, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	return &RabbitMQ{
		Conn:           conn,
		PublishChannel: publishCh,
		ConsumeChannel: consumeCh,
	}, nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.PublishChannel != nil {
		_ = r.PublishChannel.Close()
	}
	if r.ConsumeChannel != nil {
		_ = r.ConsumeChannel.Close()
	}
	if r.Conn != nil {
		return r.Conn.Close()
	}
	return nil
}

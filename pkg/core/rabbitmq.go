package core

import "github.com/rabbitmq/amqp091-go"

const (
	deadLetterExchangeSuffix = ".dlx" // 死信交换机后缀：业务交换机名 + .dlx
	deadLetterQueueSuffix    = ".dlq" // 死信队列后缀：业务队列名 + .dlq
)

type RabbitMQ struct {
	Conn           *amqp091.Connection // TCP 连接（一个）
	PublishChannel *amqp091.Channel    // 发布消息专用通道
	ConsumeChannel *amqp091.Channel    // 消费消息专用通道
}

func NewRabbitMQ(url, exchange, queue string) (*RabbitMQ, error) {
	// 1. 建立连接 + 双通道
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}
	publishCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	consumeCh, err := conn.Channel()
	if err != nil {
		_ = publishCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 2. 声明【业务交换机】
	if err := publishCh.ExchangeDeclare(
		exchange,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 3. 拼接死信名称
	dlx := exchange + deadLetterExchangeSuffix
	dlq := queue + deadLetterQueueSuffix

	// 4. 声明【死信交换机】
	if err := consumeCh.ExchangeDeclare(dlx, "fanout", true, false, false, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 5. 声明【死信队列】
	if _, err := consumeCh.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 6. 绑定：死信队列 ← 死信交换机
	if err := consumeCh.QueueBind(dlq, "", dlx, false, nil); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 7. 声明【业务队列】（挂载死信配置）
	if _, err := consumeCh.QueueDeclare(queue, true, false, false, false, amqp091.Table{
		"x-dead-letter-exchange": dlx,
	}); err != nil {
		_ = publishCh.Close()
		_ = consumeCh.Close()
		_ = conn.Close()
		return nil, err
	}

	// 8. 最后一步：绑定【业务队列 ← 业务交换机】
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

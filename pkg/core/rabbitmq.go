package core

import (
	"github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	Conn *amqp091.Connection //TCP
	//一个Conn可以开启多过分channel
	Channel *amqp091.Channel //真正手法消息
}

// 返回可用的RabbitMQ实例
func NewRabbitMQ(url, exchange, queue string) (*RabbitMQ, error) {
	//建立连接
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, err
	}
	//创建通道
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	//声明交换机
	//ch.ExchangeDeclare(
	//	exchange,  // 交换机名
	//	"fanout",  // 类型：广播模式
	//	true,      // 持久化
	//	false,     // 自动删除
	//	false,     // 内部使用
	//	false,     // 不等待
	//	nil,       // 额外参数
	//)
	if err := ch.ExchangeDeclare(exchange, "fanout", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	//声明队列
	//	ch.QueueDeclare(
	//		queue,   // 队列名
	//		true,    // 持久化
	//		false,   // 自动删除
	//		false,   // 排他
	//		false,   // 不等待
	//		nil,     // 参数
	//	)
	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	if err := ch.QueueBind(queue, "", exchange, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &RabbitMQ{
		Conn:    conn,
		Channel: ch,
	}, nil
}

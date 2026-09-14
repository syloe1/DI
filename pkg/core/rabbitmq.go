package core

// mq客户端
import (
	"context"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	deadLetterExchangeSuffix = ".dlx" // 死信交换机后缀：业务交换机名 + .dlx
	deadLetterQueueSuffix    = ".dlq" // 死信队列后缀：业务队列名 + .dlq
)

// RabbitMQ 封装连接与双通道。
// 通道通过方法按需获取（内部加锁），支持断线后原子替换，业务代码不持有过期 channel。
type RabbitMQ struct {
	mu       sync.RWMutex
	url      string
	exchange string
	queue    string

	conn      *amqp091.Connection // TCP 连接（一个）
	publishCh *amqp091.Channel    // 发布消息专用通道
	consumeCh *amqp091.Channel    // 消费消息专用通道
}

func NewRabbitMQ(url, exchange, queue string) (*RabbitMQ, error) {
	r := &RabbitMQ{url: url, exchange: exchange, queue: queue}
	if err := r.connect(); err != nil {
		return nil, err
	}
	return r, nil
}

// connect 建立连接 + 双通道 + 声明完整拓扑。
func (r *RabbitMQ) connect() error {
	// 1. 建立连接 + 双通道
	conn, err := amqp091.Dial(r.url)
	if err != nil {
		return err
	}
	publishCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}
	consumeCh, err := conn.Channel()
	if err != nil {
		_ = publishCh.Close()
		_ = conn.Close()
		return err
	}

	if err := r.declareTopology(publishCh, consumeCh); err != nil {
		_ = consumeCh.Close()
		_ = publishCh.Close()
		_ = conn.Close()
		return err
	}

	r.mu.Lock()
	r.conn = conn
	r.publishCh = publishCh
	r.consumeCh = consumeCh
	r.mu.Unlock()
	return nil
}

// declareTopology 声明业务交换机、死信交换机、死信队列、业务队列及绑定。
// 断线重连后必须重新执行（channel 失效，绑定关系需要重建）。
func (r *RabbitMQ) declareTopology(publishCh, consumeCh *amqp091.Channel) error {
	exchange := r.exchange
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
		return err
	}

	// 3. 拼接死信名称
	dlx := exchange + deadLetterExchangeSuffix
	dlq := r.queue + deadLetterQueueSuffix

	// 4. 声明【死信交换机】
	if err := consumeCh.ExchangeDeclare(dlx, "fanout", true, false, false, false, nil); err != nil {
		return err
	}

	// 5. 声明【死信队列】
	if _, err := consumeCh.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return err
	}

	// 6. 绑定：死信队列 ← 死信交换机
	if err := consumeCh.QueueBind(dlq, "", dlx, false, nil); err != nil {
		return err
	}

	// 7. 声明【业务队列】（挂载死信配置）
	if _, err := consumeCh.QueueDeclare(r.queue, true, false, false, false, amqp091.Table{
		"x-dead-letter-exchange": dlx,
	}); err != nil {
		return err
	}

	// 8. 最后一步：绑定【业务队列 ← 业务交换机】
	return consumeCh.QueueBind(r.queue, "", exchange, false, nil)
}

// PublishChan 返回当前可用的发布通道（重连后自动指向新通道）。
func (r *RabbitMQ) PublishChan() *amqp091.Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.publishCh
}

// ConsumeChan 返回当前可用的消费通道（重连后自动指向新通道）。
func (r *RabbitMQ) ConsumeChan() *amqp091.Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.consumeCh
}

// nextReconnectBackoff 指数退避：1s 起步，每次 ×2，上限 30s（避免重连风暴）。
func nextReconnectBackoff(current time.Duration) time.Duration {
	if current < time.Second {
		return time.Second
	}
	next := current * 2
	if next > 30*time.Second {
		return 30 * time.Second
	}
	return next
}

// RunReconnectLoop 监听连接关闭事件，指数退避自动重连。
// onReconnect 在重连成功后回调（用于重启 consumer）。
// 主动 Close() 时 NotifyClose 收到 nil，循环正常退出。
func (r *RabbitMQ) RunReconnectLoop(ctx context.Context, onReconnect func()) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-r.connNotifyClose():
			if err == nil {
				return // nil = 主动 Close，不重连
			}
			Z().Warnf("rabbitmq connection lost: %v, retry in %v", err, backoff)
			time.Sleep(backoff)
			backoff = nextReconnectBackoff(backoff)
			if err := r.connect(); err != nil {
				Z().Errorf("rabbitmq reconnect failed: %v", err)
				continue // 失败保持退避，继续
			}
			backoff = time.Second // 成功重置退避
			Z().Infof("rabbitmq reconnected")
			if onReconnect != nil {
				onReconnect()
			}
		}
	}
}

// connNotifyClose 安全读取当前连接的通知通道。
func (r *RabbitMQ) connNotifyClose() <-chan *amqp091.Error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.conn == nil {
		closed := make(chan *amqp091.Error)
		close(closed)
		return closed
	}
	return r.conn.NotifyClose(make(chan *amqp091.Error, 1))
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return nil
	}
	_ = r.publishCh.Close()
	_ = r.consumeCh.Close()
	err := r.conn.Close()
	r.conn = nil
	return err
}

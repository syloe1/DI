package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go-admin/config"
	"go-admin/pkg/core"

	"github.com/rabbitmq/amqp091-go"
)

// 查死信消息， 重新投递死信消息
func main() {
	var (
		configPath = flag.String("config", "config/config.yaml", "config file path")
		queue      = flag.String("queue", "", "base queue name")
		instanceID = flag.String("instance-id", "", "instance id")
		dlqName    = flag.String("dlq", "", "full dlq name")
		mode       = flag.String("mode", "list", "list or replay")
		limit      = flag.Int("limit", 20, "max messages to process, 0 means all available")
	)
	flag.Parse()
	//限制工具只能运行在 list（查看死信） 和 replay（重放死信）
	if *mode != "list" && *mode != "replay" {
		log.Fatalf("invalid mode: %s", *mode)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	//确定业务队列名
	fullQueue := strings.TrimSpace(*queue)
	if fullQueue == "" {
		fullQueue = cfg.RabbitMQ.Queue
	}
	//确认实例ID
	fullInstanceID := strings.TrimSpace(*instanceID)
	if fullInstanceID == "" {
		fullInstanceID = resolveInstanceID()
	}

	fullDLQ := strings.TrimSpace(*dlqName)
	if fullDLQ == "" {
		fullDLQ = fmt.Sprintf("%s.%s.dlq", fullQueue, fullInstanceID)
	}
	//初始化并连接 RabbitMQ
	rabbit, err := core.NewRabbitMQ(cfg.RabbitMQ.URL, cfg.RabbitMQ.Exchange, fmt.Sprintf("%s.%s", fullQueue, fullInstanceID))
	if err != nil {
		log.Fatalf("connect rabbitmq failed: %v", err)
	}
	defer func() { _ = rabbit.Close() }()

	ctx := context.Background()
	switch *mode {
	case "list":
		if err := listDLQ(ctx, rabbit.ConsumeChannel, fullDLQ, *limit); err != nil {
			log.Fatalf("list dlq failed: %v", err)
		}
	case "replay":
		if err := replayDLQ(ctx, rabbit.ConsumeChannel, rabbit.PublishChannel, cfg.RabbitMQ.Exchange, fullDLQ, *limit); err != nil {
			log.Fatalf("replay dlq failed: %v", err)
		}
	}
}

// 遍历并打印死信队列消息，仅查看、不删除原消息
func listDLQ(ctx context.Context, ch *amqp091.Channel, dlq string, limit int) error {
	count := 0
	for {
		if limit > 0 && count >= limit {
			return nil
		}
		// 达到最大查看条数，退出循环
		msg, ok, err := ch.Get(dlq, false)
		if err != nil {
			return err
		}
		// ok=false 说明队列已空，结束查看
		if !ok {
			return nil
		}

		printDeliverySummary(count+1, msg)
		// Nack：拒绝签收，requeue=true → 消息重新放回原死信队列
		_ = msg.Nack(false, true)
		count++
	}
}

func replayDLQ(ctx context.Context, consumeCh *amqp091.Channel, publishCh *amqp091.Channel, exchange, dlq string, limit int) error {
	count := 0
	for {
		// 达到指定处理条数，退出
		if limit > 0 && count >= limit {
			return nil
		}
		// 从死信队列单条拉取消息，autoAck=false 手动应答
		msg, ok, err := consumeCh.Get(dlq, false)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		// 拷贝消息结构（过滤死信专属头部、保留业务数据）
		body := clonePublishing(msg)
		// 重新发布到业务交换机
		if err := publishCh.PublishWithContext(ctx, exchange, "", false, false, body); err != nil {
			// 发布失败：Nack + 重新入队，消息留在死信队列，后续可再次重试
			_ = msg.Nack(false, true)
			log.Printf("publish message failed, continue next. err: %v", err)
			continue
		}
		// 发布成功：手动ACK，从死信队列彻底删除该消息
		if err := msg.Ack(false); err != nil {
			log.Printf("ack message failed, may cause duplicate msg. err: %v", err)
			return err
		}
		// 打印重放日志
		fmt.Printf("[replayed] #%d message_id=%s request_id=%s type=%s group_id=%d\n",
			count+1, getHeaderString(msg.Headers, "message_id"), getHeaderString(msg.Headers, "request_id"), getHeaderString(msg.Headers, "type"), getHeaderUint(msg.Headers, "group_id"))
		count++
	}
}

func printDeliverySummary(index int, msg amqp091.Delivery) {
	body := strings.TrimSpace(string(msg.Body))
	if len(body) > 512 {
		body = body[:512] + "..."
	}
	fmt.Printf("[%d] message_id=%s request_id=%s type=%s group_id=%d redelivered=%t\n",
		index,
		getHeaderString(msg.Headers, "message_id"),
		getHeaderString(msg.Headers, "request_id"),
		getHeaderString(msg.Headers, "type"),
		getHeaderUint(msg.Headers, "group_id"),
		msg.Redelivered,
	)
	fmt.Printf("    body=%s\n", body)
}

// 将从死信队列取出的 Delivery 消息，复制转换成可重新发布的 Publishing 结构
func clonePublishing(msg amqp091.Delivery) amqp091.Publishing {
	//跳过 x-death
	headers := amqp091.Table{}
	for k, v := range msg.Headers {
		if strings.EqualFold(k, "x-death") {
			continue
		}
		headers[k] = v
	}

	return amqp091.Publishing{
		ContentType:     msg.ContentType,     // 内容类型（如 application/json）
		ContentEncoding: msg.ContentEncoding, // 内容编码
		DeliveryMode:    amqp091.Persistent,  // 消息持久化：强制设为持久化，防止服务重启丢失
		Priority:        msg.Priority,        // 消息优先级
		CorrelationId:   msg.CorrelationId,   // 关联ID（常用于RPC调用）
		ReplyTo:         msg.ReplyTo,         // 回调队列
		Expiration:      msg.Expiration,      // 消息过期时间
		MessageId:       msg.MessageId,       // 消息唯一ID
		Timestamp:       time.Now(),          // 重置为当前时间（重放时间）
		Type:            msg.Type,            // 消息业务类型
		UserId:          msg.UserId,          // 发送用户标识
		AppId:           msg.AppId,           // 发送应用标识
		Headers:         headers,             // 处理后的头部（已剔除 x-death）
		Body:            msg.Body,            // 完整原始消息体
	}
}

func getHeaderString(headers amqp091.Table, key string) string {
	// 头部为空，直接返回空串
	if headers == nil {
		return ""
	}
	// 找不到对应 key，返回空串
	value, ok := headers[key]
	if !ok {
		return ""
	}

	// 类型分支适配
	switch v := value.(type) {
	case string: // 本身就是字符串，直接返回
		return v
	case []byte: // 字节数组，转字符串
		return string(v)
	case fmt.Stringer: // 实现了 String() 接口的对象，调用方法转字符串
		return v.String()
	default: // 其它任意类型，格式化转为字符串
		return fmt.Sprintf("%v", v)
	}
}

func getHeaderUint(headers amqp091.Table, key string) uint64 {
	if headers == nil {
		return 0
	}
	value, ok := headers[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case uint8:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint32:
		return uint64(v)
	case uint64:
		return v
	case int:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case int64:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case float64:
		if v < 0 {
			return 0
		}
		return uint64(v)
	case string:
		var n uint64
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	default:
		return 0
	}
}

func resolveInstanceID() string {
	// 1. 优先读取环境变量 INSTANCE_ID
	if val := os.Getenv("INSTANCE_ID"); val != "" {
		return val
	}

	// 2. 环境变量不存在时，获取本机主机名
	hostname, err := os.Hostname()
	// 主机名获取失败/为空，兜底设为 unknown
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	// 3. 拼接：主机名 + 当前进程PID，作为实例ID返回
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

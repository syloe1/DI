package core

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type ChatMetrics struct {
	OnlineConnections   atomic.Int64 // 当前在线连接数（实时）
	PushDelivered       atomic.Int64 // 推送成功总数
	PushDroppedSlow     atomic.Int64 // 因下游处理过慢被丢弃的推送消息数
	ConsumerSucceeded   atomic.Int64 // 消息消费成功累计数
	ConsumerFailed      atomic.Int64 // 消息消费失败累计数
	OutboxPublished     atomic.Int64 // 发件箱消息发布成功累计数
	OutboxPublishFailed atomic.Int64 // 发件箱消息发布失败累计数
	OutboxPending       atomic.Int64 // 发件箱待处理积压数
	OutboxProcessing    atomic.Int64 // 发件箱当前处理中数量
	OutboxFailed        atomic.Int64 // 发件箱处理失败数量
}

var Metrics ChatMetrics

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "chat_online_connections %d\n", Metrics.OnlineConnections.Load())
	_, _ = fmt.Fprintf(w, "chat_push_delivered_total %d\n", Metrics.PushDelivered.Load())
	_, _ = fmt.Fprintf(w, "chat_push_dropped_slow_total %d\n", Metrics.PushDroppedSlow.Load())
	_, _ = fmt.Fprintf(w, "chat_consumer_succeeded_total %d\n", Metrics.ConsumerSucceeded.Load())
	_, _ = fmt.Fprintf(w, "chat_consumer_failed_total %d\n", Metrics.ConsumerFailed.Load())
	_, _ = fmt.Fprintf(w, "chat_outbox_published_total %d\n", Metrics.OutboxPublished.Load())
	_, _ = fmt.Fprintf(w, "chat_outbox_publish_failed_total %d\n", Metrics.OutboxPublishFailed.Load())
	_, _ = fmt.Fprintf(w, "chat_outbox_pending %d\n", Metrics.OutboxPending.Load())
	_, _ = fmt.Fprintf(w, "chat_outbox_processing %d\n", Metrics.OutboxProcessing.Load())
	_, _ = fmt.Fprintf(w, "chat_outbox_failed %d\n", Metrics.OutboxFailed.Load())
}

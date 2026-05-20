package core

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type ChatMetrics struct {
	OnlineConnections   atomic.Int64
	PushDelivered       atomic.Int64
	PushDroppedSlow     atomic.Int64
	ConsumerSucceeded   atomic.Int64
	ConsumerFailed      atomic.Int64
	OutboxPublished     atomic.Int64
	OutboxPublishFailed atomic.Int64
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
}

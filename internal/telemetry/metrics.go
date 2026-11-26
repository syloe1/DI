package telemetry

// 轻量级的指标跟踪系统
import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
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

	HTTPRequests atomic.Int64 // HTTP 请求总数
	HTTPErrors   atomic.Int64 // 4xx/5xx 错误数
}

var Metrics ChatMetrics

// 延迟直方图：固定分桶（毫秒上界），路由模板级、避免高基数用户 ID 标签
var latencyBucketUpperMs = []int64{10, 25, 50, 100, 250, 500, 1000}

type routeHistogram struct {
	count   atomic.Int64
	buckets [8]atomic.Int64 // 前 7 个对应上界，最后一个 >= 1000ms
}

var routeHistograms sync.Map // route template -> *routeHistogram

// RecordHTTPRequest 记录一次 HTTP 请求的延迟分布与状态码。
func RecordHTTPRequest(route string, status int, latency time.Duration) {
	Metrics.HTTPRequests.Add(1)
	if status >= 400 {
		Metrics.HTTPErrors.Add(1)
	}
	if route == "" {
		route = "unknown"
	}

	hist, _ := routeHistograms.LoadOrStore(route, &routeHistogram{})
	h := hist.(*routeHistogram)
	h.count.Add(1)

	ms := latency.Milliseconds()
	for i, upper := range latencyBucketUpperMs {
		if ms < upper {
			h.buckets[i].Add(1)
			return
		}
	}
	h.buckets[len(latencyBucketUpperMs)].Add(1)
}

// latencyPercentile 从分桶直方图近似计算分位数（返回毫秒上界）。
func latencyPercentile(h *routeHistogram, p float64) int64 {
	total := h.count.Load()
	if total == 0 {
		return 0
	}
	target := int64(float64(total) * p)
	var cum int64
	for i, upper := range latencyBucketUpperMs {
		cum += h.buckets[i].Load()
		if cum >= target {
			return upper
		}
	}
	return latencyBucketUpperMs[len(latencyBucketUpperMs)-1]
}

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
	_, _ = fmt.Fprintf(w, "chat_http_requests_total %d\n", Metrics.HTTPRequests.Load())
	_, _ = fmt.Fprintf(w, "chat_http_errors_total %d\n", Metrics.HTTPErrors.Load())

	// 路由级延迟直方图（稳定排序，便于对比）
	routes := make([]string, 0)
	routeHistograms.Range(func(key, _ interface{}) bool {
		routes = append(routes, key.(string))
		return true
	})
	sort.Strings(routes)
	for _, route := range routes {
		hist, _ := routeHistograms.Load(route)
		h := hist.(*routeHistogram)
		_, _ = fmt.Fprintf(w, "chat_http_latency_ms route=%q count=%d p50=%d p95=%d p99=%d\n",
			route, h.count.Load(),
			latencyPercentile(h, 0.50), latencyPercentile(h, 0.95), latencyPercentile(h, 0.99))
	}
}

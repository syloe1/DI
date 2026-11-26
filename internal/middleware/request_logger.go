package middleware

import (
	"time"

	"di/internal/telemetry"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLogger(logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()        // 请求进入瞬间记录开始时间
		method := c.Request.Method // GET/POST/PUT
		clientIP := c.ClientIP()   // 请求客户端IP

		c.Next() // 放行：先走鉴权、限流、业务handler，全部执行完再往下打日志

		// 只记录 Gin 路由模板（如 /api/users/:id），不记录原始 path + query：
		// ?token= 等认证参数会随 RawQuery 进入应用日志/代理日志/浏览器历史。
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path // 未匹配路由（如 404）时退回 path（不含 query）
		}
		status := c.Writer.Status()
		latency := time.Since(start)

		// 统一格式化输出日志（zap 结构化字段）
		logger.Infow("http request",
			"request_id", GetRequestID(c), // 全链路追踪ID（依赖上个中间件RequestID）
			"method", method,
			"path", path,
			"status", status, // http响应码 200/401/429/500
			"latency", latency, // 接口耗时
			"client_ip", clientIP,
			"user_id", c.GetUint("userID"), // jwt解析出来的登录用户id，未登录是0
		)

		// 路由模板级延迟直方图（P50/P95/P99），供 /metrics 观测
		telemetry.RecordHTTPRequest(path, status, latency)
	}
}

package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()                // 请求进入瞬间记录开始时间
		method := c.Request.Method         // GET/POST/PUT
		path := c.Request.URL.Path         // 接口路由 /api/user/login
		rawQuery := c.Request.URL.RawQuery // url拼接参数 ?id=1&name=xx
		clientIP := c.ClientIP()           // 请求客户端IP

		c.Next() // 放行：先走鉴权、限流、业务handler，全部执行完再往下打日志

		// 有url参数就拼接完整路径
		if rawQuery != "" {
			path += "?" + rawQuery
		}

		// 统一格式化输出日志
		logger.Printf(
			"request_id=%s method=%s path=%s status=%d latency=%s client_ip=%s user_id=%d",
			GetRequestID(c), // 全链路追踪ID（依赖上个中间件RequestID）
			method,
			path,
			c.Writer.Status(), // http响应码 200/401/429/500
			time.Since(start), // 接口耗时
			clientIP,
			c.GetUint("userID"), // jwt解析出来的登录用户id，未登录是0
		)
	}
}

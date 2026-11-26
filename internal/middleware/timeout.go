package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeout 给每个请求设置 deadline（context.WithTimeout）。
// 客户端取消或超时后，透传 ctx 的 DB/Redis/MQ 操作会被及时取消，避免慢请求长期占用资源。
func RequestTimeout(timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

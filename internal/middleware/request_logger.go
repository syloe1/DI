package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func RequestLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method := c.Request.Method
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery
		clientIP := c.ClientIP()

		c.Next()

		if rawQuery != "" {
			path += "?" + rawQuery
		}

		logger.Printf(
			"request_id=%s method=%s path=%s status=%d latency=%s client_ip=%s user_id=%d",
			GetRequestID(c),
			method,
			path,
			c.Writer.Status(),
			time.Since(start),
			clientIP,
			c.GetUint("userID"),
		)
	}
}

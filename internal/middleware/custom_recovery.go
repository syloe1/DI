package middleware

import (
	"net/http"

	"di/internal/transport"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func CustomRecovery(logger *zap.SugaredLogger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {

		// 1. 打印崩溃日志（zap 结构化字段）
		logger.Errorw("http panic recovered",
			"request_id", GetRequestID(c), // 请求ID（追踪）
			"method", c.Request.Method, // GET/POST
			"path", c.Request.URL.Path, // 接口地址
			"status", http.StatusInternalServerError, // 500
			"client_ip", c.ClientIP(), // 访问者IP
			"user_id", c.GetUint("userID"), // 哪个用户崩的
			"panic", recovered, // 崩溃具体错误
		)

		// 2. 给前端返回统一错误
		transport.Fail(c, http.StatusInternalServerError, "服务器内部错误")
	})
}

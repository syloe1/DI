package middleware

import (
	"log"
	"net/http"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

func CustomRecovery(logger *log.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {

		// 1. 打印崩溃日志（超级详细）
		logger.Printf(
			"request_id=%s method=%s path=%s status=%d client_ip=%s user_id=%d panic=%v",
			GetRequestID(c),                // 请求ID（追踪）
			c.Request.Method,               // GET/POST
			c.Request.URL.Path,             // 接口地址
			http.StatusInternalServerError, // 500
			c.ClientIP(),                   // 访问者IP
			c.GetUint("userID"),            // 哪个用户崩的
			recovered,                      // 崩溃具体错误
		)

		// 2. 给前端返回统一错误
		core.Fail(c, http.StatusInternalServerError, "服务器内部错误")
	})
}

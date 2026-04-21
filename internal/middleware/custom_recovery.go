package middleware

import (
	"log"
	"net/http"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

func CustomRecovery(logger *log.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		logger.Printf(
			"request_id=%s method=%s path=%s status=%d client_ip=%s user_id=%d panic=%v",
			GetRequestID(c),
			c.Request.Method,
			c.Request.URL.Path,
			http.StatusInternalServerError,
			c.ClientIP(),
			c.GetUint("userID"),
			recovered,
		)

		core.Fail(c, http.StatusInternalServerError, "服务器内部错误")
	})
}

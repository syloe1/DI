package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Cors 白名单 CORS 中间件（P0-2 修复）：
// 仅当请求 Origin 命中白名单时回显该 Origin 并允许携带凭证；
// 未命中一律不设置 Allow-Origin（浏览器会拦截），避免反射 Origin + Credentials 的 CSRF/凭证滥用风险。
func Cors(allowOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowOrigins))
	for _, o := range allowOrigins {
		if o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Token, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent) // 预检请求直接返回
			return
		}
		c.Next()
	}
}

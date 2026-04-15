package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" && role != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{
			"code": 403,
			"msg":  "无权限",
		})
			c.Abort()
			return
		}
		c.Next()
	}
}

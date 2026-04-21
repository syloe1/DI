package middleware

import (
	"net/http"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" && role != "superadmin" {
			core.Fail(c, http.StatusForbidden, "无权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

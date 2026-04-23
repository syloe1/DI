package middleware

import (
	"net/http"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

// AdminAuth 管理员权限校验中间件
// 用于校验当前登录用户是否为管理员/超级管理员，只有管理员角色才能访问接口
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Gin 上下文中获取当前登录用户的角色
		// 该值由前面的 JWTAuth 中间件解析 Token 后存入
		role := c.GetString("role")

		// 判断角色是否为 admin 管理员 或 superadmin 超级管理员
		if role != "admin" && role != "superadmin" {
			// 不是管理员，返回无权限错误
			core.Fail(c, http.StatusForbidden, "无权限")
			c.Abort() // 中断请求，不再执行后续接口
			return
		}

		// 是管理员，继续执行后续接口
		c.Next()
	}
}

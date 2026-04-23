package middleware

import (
	"net/http"
	"strings"

	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

// Claims 自定义JWT载荷结构体
// 存储Token中包含的用户信息，用于接口鉴权
type Claims struct {
	UserId               uint   `json:"userId"`   // 用户ID
	Username             string `json:"username"` // 用户名
	Role                 string `json:"role"`     // 用户角色
	jwt.RegisteredClaims        // JWT标准声明（过期时间、签发时间等）
}

// JWTAuth JWT身份验证中间件
// 用于保护需要登录才能访问的接口，验证请求头中的Token是否合法
// 参数secret：用于验证Token签名的密钥
func JWTAuth(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取请求头中的 Authorization 字段
		authHeader := c.GetHeader("Authorization")

		// 2. 判断请求头是否为空或格式不正确（必须以 Bearer 开头）
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			core.Fail(c, http.StatusUnauthorized, "未登录")
			c.Abort() // 中断请求
			return
		}

		// 3. 提取纯Token字符串（去掉 "Bearer " 前缀）
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// 4. 初始化自定义Claims结构体，用于解析后存储用户信息
		claims := &Claims{}

		// 5. 解析并验证Token
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// 验证签名算法是否为HMAC（防止加密算法被篡改）
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			// 返回密钥用于验证签名
			return secret, nil
		})

		// 6. 解析出错或Token无效
		if err != nil || !token.Valid {
			core.Fail(c, http.StatusUnauthorized, "token无效")
			c.Abort()
			return
		}

		// 7. 将解析出的用户信息存入上下文，后续接口可直接使用
		c.Set("userID", claims.UserId)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		// 8. 验证通过，继续执行后续请求
		c.Next()
	}
}

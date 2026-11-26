package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"di/internal/telemetry"

	"di/internal/transport"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

// Claims 自定义JWT载荷结构体
// 存储Token中包含的用户信息，用于接口鉴权
type Claims struct {
	UserId   uint   `json:"user_id"`  // 用户ID
	Username string `json:"username"` // 用户名
	Role     string `json:"role"`     // 用户角色
	// IssuedAtMs 毫秒级签发时间（标准 iat 只有秒级，改密吊销同秒 token 会漏）
	IssuedAtMs           int64 `json:"iat_ms,omitempty"`
	jwt.RegisteredClaims       // JWT标准声明（过期时间、签发时间等）
}

// TokenRevoker JWT 主动失效检查能力（service.TokenRevocationStore 实现）。
// 中间件只依赖接口，避免 middleware -> service 的反向依赖。
type TokenRevoker interface {
	IsRevoked(ctx context.Context, token string, userID uint, issuedAt time.Time) (bool, error)
	Revoke(ctx context.Context, token string, userID uint, expiresAt time.Time) error
}

// issuedAt 返回 token 的签发时间（优先毫秒级 iat_ms；老 token 退回 iat 秒级）。
func (c *Claims) issuedAt() time.Time {
	if c.IssuedAtMs > 0 {
		return time.UnixMilli(c.IssuedAtMs)
	}
	if c.IssuedAt == nil {
		return time.Time{}
	}
	return c.IssuedAt.Time
}

// JWTAuth JWT身份验证中间件
// 用于保护需要登录才能访问的接口，验证请求头中的Token是否合法。
// 参数 secret：用于验证Token签名的密钥；revoker：主动失效检查（可为 nil）。
func JWTAuth(secret []byte, revoker TokenRevoker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 获取请求头中的 Authorization 字段
		authHeader := c.GetHeader("Authorization")

		// 2. 判断请求头是否为空或格式不正确（必须以 Bearer 开头）
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			transport.Fail(c, http.StatusUnauthorized, "未登录")
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
			transport.Fail(c, http.StatusUnauthorized, "token无效")
			c.Abort()
			return
		}

		// 7. 主动失效检查：登出后的 token 立即 401
		if revoker != nil {
			revoked, checkErr := revoker.IsRevoked(c.Request.Context(), tokenString, claims.UserId, claims.issuedAt())
			if checkErr != nil {
				// Redis 抖动时 fail-open（可用性优先），仅记录告警；正常路径不会走到这里。
				telemetry.Z().Warnf("check token revocation failed: %v", checkErr)
			} else if revoked {
				transport.Fail(c, http.StatusUnauthorized, "登录状态已失效，请重新登录")
				c.Abort()
				return
			}
		}

		// 8. 将解析出的用户信息存入上下文，后续接口可直接使用
		c.Set("userID", claims.UserId)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		// 登出时要把当前 token 加黑名单，需要原始串与过期时间
		c.Set("token", tokenString)
		if claims.ExpiresAt != nil {
			c.Set("tokenExpiresAt", claims.ExpiresAt.Time)
		}

		// 9. 验证通过，继续执行后续请求
		c.Next()
	}
}

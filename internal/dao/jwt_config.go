package dao

import "time"

// DefaultJWTConfig 默认的 JWT 配置实现
// 用于存储和提供 JWT 签名密钥与有效期
type DefaultJWTConfig struct {
	Secret      []byte        // JWT 签名密钥，用于签发和验证 Token
	ExpireHours time.Duration // Token 有效期
}

// GetSecret 获取 JWT 签名密钥
// 实现 JWTConfig 接口方法
func (d *DefaultJWTConfig) GetSecret() []byte {
	return d.Secret
}

// GetExpireHours 获取 Token 有效期
// 实现 JWTConfig 接口方法
func (d *DefaultJWTConfig) GetExpireHours() time.Duration {
	return d.ExpireHours
}

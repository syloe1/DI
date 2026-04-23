package dao

// DefaultJWTConfig 默认的 JWT 配置实现
// 用于存储和提供 JWT 签名密钥
type DefaultJWTConfig struct {
	Secret []byte // JWT 签名密钥，用于签发和验证 Token
}

// GetSecret 获取 JWT 签名密钥
// 实现 JWTConfig 接口方法
func (d *DefaultJWTConfig) GetSecret() []byte {
	return d.Secret
}

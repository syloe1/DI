package repository

// DefaultJWTConfig 默认的JWT配置实现
type DefaultJWTConfig struct {
	Secret []byte
}

func (d *DefaultJWTConfig) GetSecret() []byte {
	return d.Secret
}
package dao

// DefaultJWTConfig 榛樿鐨凧WT閰嶇疆瀹炵幇
type DefaultJWTConfig struct {
	Secret []byte
}

func (d *DefaultJWTConfig) GetSecret() []byte {
	return d.Secret
}

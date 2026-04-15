package repository

import (
	"context"
	"go-admin/model"
	"time"
)

// 数据库操作接口
type UserDB interface {
	Create(user *model.User) error
	FindByID(id uint) (*model.User, error)
	FindByUsername(username string) (*model.User, error)
	Update(user *model.User) error
	Delete(id uint) error
	FindAll() ([]model.User, error)
	FindByIDs(ids []uint) ([]model.User, error)
	CountByUsername(username string) (int64, error)
	FindByUsernameLike(username string, limit int) ([]model.User, error)
}

// 缓存操作接口
type UserCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// 响应结构体
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// JWT配置接口
type JWTConfig interface {
	GetSecret() []byte
}

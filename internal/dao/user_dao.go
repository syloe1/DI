package dao

import (
	"context"
	"time"

	"go-admin/internal/domain/model"
)

type ZSetMemberScore struct {
	Member string
	Score  float64
}

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

type UserCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) (bool, error)
	Incr(ctx context.Context, key string) (int64, error)
	HSet(ctx context.Context, key string, values map[string]interface{}) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error)
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZSetMemberScore, error)
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SRem(ctx context.Context, key string, members ...interface{}) error
	SIsMember(ctx context.Context, key string, member interface{}) (bool, error)
	SMembers(ctx context.Context, key string) ([]string, error)
	Del(ctx context.Context, keys ...string) error
}

type JWTConfig interface {
	GetSecret() []byte
}

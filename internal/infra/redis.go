package infra

import (
	"context"
	"di/internal/config"
	"fmt"

	"github.com/go-redis/redis/v8"
)

func InitRedis(cfg config.RedisConfig) (*redis.Client, error) {
	if cfg.Host == "" || cfg.Port == "" {
		return nil, fmt.Errorf("redis host or port is empty")
	}

	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = 32
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.Db,
		PoolSize: poolSize,
	})
	// 发 ping 确认 Redis 可达
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

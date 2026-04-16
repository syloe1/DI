package core

import (
	"context"
	"fmt"
	"go-admin/config"

	"github.com/go-redis/redis/v8"
)

func InitRedis(cfg config.RedisConfig) (*redis.Client, error) {
	if cfg.Host == "" || cfg.Port == "" {
		return nil, fmt.Errorf("redis host or port is empty")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password,
		DB:       cfg.Db,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

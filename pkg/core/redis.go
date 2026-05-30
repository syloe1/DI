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
		Addr:     cfg.Host + ":" + cfg.Port, // 拼接地址：127.0.0.1:6379
		Password: cfg.Password,              // Redis 密码（没有就为空）
		DB:       cfg.Db,                    // Redis 库（0~15）
	})
	//发ping确认Redis成功启动
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

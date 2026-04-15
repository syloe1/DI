package core

import (
	"context"
	"go-admin/config"
	"log"

	"github.com/go-redis/redis/v8"
)

// 封装redis客户端
var RDB *redis.Client
var Ctx = context.Background()

func InitRedis() {
	if config.GlobalConfig == nil {
		log.Fatal("请先初始化全局配置(调用config.InitGlobalConfig())")
	}
	redisCfg := config.GlobalConfig.GetRedisConfig()

	if redisCfg.Host == "" || redisCfg.Port == "" {
		log.Fatal("Redis主机或端口为空")
	}
	RDB = redis.NewClient(&redis.Options{
		Addr:     redisCfg.Host + ":" + redisCfg.Port,
		Password: redisCfg.Password,
		DB:       redisCfg.Db,
	})
	_, err := RDB.Ping(Ctx).Result()
	if err != nil {
		log.Fatal("Redis connected fail: ", err)
	}
	log.Println("✅ Redis连接成功")
}

func CloseRedis() {
	if RDB != nil {
		if err := RDB.Close(); err != nil {
			log.Fatalf("redis 关闭失败： %v", err)
		} else {
			log.Println("redis 连接已关闭")
		}
	}
}

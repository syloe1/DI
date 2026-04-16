package main

import (
	"go-admin/config"
	"go-admin/core"
	"go-admin/internal/container"
	"go-admin/router"
)

func main() {
	appLogger := core.NewLogger()

	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		appLogger.Fatalf("load config failed: %v", err)
	}

	db, err := core.InitMysql(cfg.GetMysqlConfig())
	if err != nil {
		appLogger.Fatalf("init mysql failed: %v", err)
	}

	redisClient, err := core.InitRedis(cfg.GetRedisConfig())
	if err != nil {
		appLogger.Fatalf("init redis failed: %v", err)
	}
	defer redisClient.Close()

	if err := core.AutoMigrate(db); err != nil {
		appLogger.Fatalf("auto migrate failed: %v", err)
	}

	appContainer := container.NewContainer(cfg, db, redisClient, appLogger)
	appLogger.Println("DI container initialized")

	r := router.InitDependencyInjectionRouter(appContainer)
	if err := r.Run(":" + cfg.GetServerConfig().Port); err != nil {
		appLogger.Fatalf("start server failed: %v", err)
	}
}

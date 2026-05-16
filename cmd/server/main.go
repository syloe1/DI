package main

import (
	"go-admin/config"
	"go-admin/internal/container"
	"go-admin/internal/router"
	"go-admin/pkg/core"
	"log"
	"net/http"
	_ "net/http/pprof" // 只需要匿名导入
)

func main() {
	// ========== pprof 性能分析（后台独立运行）==========
	go func() {
		log.Println("✅ pprof 已启动: http://127.0.0.1:6060/debug/pprof")
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			log.Fatalf("pprof 启动失败: %v", err)
		}
	}()
	appLogger := core.NewLogger()

	if err := core.RegisterCustomValidators(); err != nil {
		appLogger.Fatalf("register custom validators failed: %v", err)
	}

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

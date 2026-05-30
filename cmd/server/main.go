package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"

	"go-admin/config"
	"go-admin/internal/container"
	"go-admin/internal/router"
	"go-admin/pkg/core"
)

func main() {
	go func() {
		log.Println("pprof started: http://127.0.0.1:6060/debug/pprof")
		http.HandleFunc("/metrics", core.MetricsHandler)
		if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
			log.Fatalf("pprof start failed: %v", err)
		}
	}()

	appLogger := core.NewLogger()
	//注册自定义参数校验器
	if err := core.RegisterCustomValidators(); err != nil {
		appLogger.Fatalf("register custom validators failed: %v", err)
	}
	// 加载配置文件
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
	defer func() { _ = redisClient.Close() }()
	//异步数据库迁移
	go func() {
		appLogger.Print("start database auto migration")
		if err := core.AutoMigrate(db); err != nil {
			appLogger.Printf("database auto migration failed: %v", err)
			return
		}
		appLogger.Print("database auto migration completed")
	}()
	//依赖注入容器 DI Container
	appContainer := container.NewContainer(cfg, db, redisClient, appLogger)
	appLogger.Println("DI container initialized")
	//初始化路由
	r := router.InitDependencyInjectionRouter(appContainer)
	appLogger.Printf("server started, port: %s", cfg.GetServerConfig().Port)

	if err := r.Run(":" + cfg.GetServerConfig().Port); err != nil {
		appLogger.Fatalf("start server failed: %v", err)
	}
}

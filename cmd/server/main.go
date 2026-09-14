package main

import (
	"context"
	"errors"
	"net/http"
	_ "net/http/pprof"
	"os/signal"
	"syscall"
	"time"

	"go-admin/config"
	"go-admin/internal/container"
	"go-admin/internal/router"
	"go-admin/pkg/core"
)

func main() {
	// 加载配置文件（未初始化 logger 前，失败用 core.Z() 兜底输出）
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		core.Z().Fatalf("load config failed: %v", err)
	}

	// 初始化结构化日志（zap）：级别/格式来自配置，console 开发 / json 生产
	if _, err := core.InitLogger(cfg.GetLoggerConfig().Level, cfg.GetLoggerConfig().Format); err != nil {
		core.Z().Fatalf("init logger failed: %v", err)
	}
	appLogger := core.Z()
	defer core.SyncLogger()

	//注册自定义参数校验器
	if err := core.RegisterCustomValidators(); err != nil {
		appLogger.Fatalf("register custom validators failed: %v", err)
	}

	db, err := core.InitMysql(cfg.GetMysqlConfig())
	if err != nil {
		appLogger.Fatalf("init mysql failed: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		appLogger.Fatalf("get sql db failed: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	redisClient, err := core.InitRedis(cfg.GetRedisConfig())
	if err != nil {
		appLogger.Fatalf("init redis failed: %v", err)
	}
	defer func() { _ = redisClient.Close() }()
	//异步数据库迁移（golang-migrate，SQL 版本化）
	go func() {
		appLogger.Info("start database migration")
		if err := core.RunMigrations(db); err != nil {
			appLogger.Errorf("database migration failed: %v", err)
			return
		}
		appLogger.Info("database migration completed")
	}()
	//依赖注入容器 DI Container
	appContainer := container.NewContainer(cfg, db, redisClient, appLogger)
	appLogger.Info("DI container initialized")
	//初始化路由
	r := router.InitDependencyInjectionRouter(appContainer)

	// 优雅关闭：信号 → 触发清理序列
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 主 HTTP 服务（gin engine 即 http.Handler）
	srv := &http.Server{
		Addr:    ":" + cfg.GetServerConfig().Port,
		Handler: r,
	}
	go func() {
		appLogger.Infof("server started, port: %s", cfg.GetServerConfig().Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatalf("start server failed: %v", err)
		}
	}()

	// pprof / metrics 独立监听（保留原行为）
	metricsSrv := &http.Server{Addr: "127.0.0.1:6060", Handler: http.DefaultServeMux}
	go func() {
		appLogger.Info("pprof started: http://127.0.0.1:6060/debug/pprof")
		http.HandleFunc("/metrics", core.MetricsHandler)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Errorf("pprof start failed: %v", err)
		}
	}()

	// 阻塞等待退出信号
	<-ctx.Done()
	appLogger.Info("shutting down...")

	// 优雅关闭：每步带超时，防止等待排空卡死
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 停 HTTP：不再接收新连接，等待存量请求处理完
	_ = srv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)
	// 2. 停 WS / 清在线状态 / 停 outbox+consumer / 关 RabbitMQ
	appContainer.Shutdown(shutdownCtx)
	appLogger.Info("shutdown completed")
}

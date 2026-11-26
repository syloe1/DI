package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"di/internal/config"
	"di/internal/container"
	"di/internal/infra"
	"di/internal/router"
	"di/internal/telemetry"
	"di/internal/transport"
)

// main 保持极简：只处理信号、调用run，不写业务初始化
func main() {
	// 监听系统退出信号
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		// 这里logger还没初始化完成时，fallback到stderr输出
		_, _ = os.Stderr.WriteString("server run failed: " + err.Error() + "\n")
		os.Exit(1)
	}
}

// run 所有服务启动逻辑全部放在这里
func run(ctx context.Context) error {
	// 1.加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return err
	}

	// 2.初始化日志
	_, err = telemetry.InitLogger(cfg.GetLoggerConfig().Level, cfg.GetLoggerConfig().Format)
	if err != nil {
		return err
	}
	appLogger := telemetry.Z()
	defer telemetry.SyncLogger()

	// 3.注册校验器
	if err := transport.RegisterCustomValidators(); err != nil {
		return err
	}

	// 4.初始化Mysql
	db, err := infra.InitMysql(cfg.GetMysqlConfig())
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	// 5.初始化Redis
	redisClient, err := infra.InitRedis(cfg.GetRedisConfig())
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	// 6.数据库迁移（同步执行：迁移完成前不监听端口，失败则拒绝启动，避免竞态）
	appLogger.Info("start database migration")
	if err := infra.RunMigrations(db); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	appLogger.Info("database migration completed")

	// 7.构建DI容器、路由
	appContainer, err := container.InitializeContainer(cfg, db, redisClient, appLogger)
	if err != nil {
		return err
	}
	appLogger.Info("DI container initialized")
	r := router.InitDependencyInjectionRouter(appContainer)

	// 8.构建http服务（应用配置的读写超时；WS 升级后连接被 hijack，不受这些 deadline 约束）
	serverCfg := cfg.GetServerConfig()
	srv := &http.Server{
		Addr:              ":" + serverCfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       serverCfg.ReadTimeout,
		WriteTimeout:      serverCfg.WriteTimeout,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	metricsSrv := &http.Server{Addr: "127.0.0.1:6060", Handler: http.DefaultServeMux}
	http.HandleFunc("/metrics", telemetry.MetricsHandler)

	// 启动业务服务
	go func() {
		appLogger.Infof("server started, port: %s", cfg.GetServerConfig().Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatalf("start server failed: %v", err)
		}
	}()

	// 启动pprof metrics服务
	go func() {
		appLogger.Info("pprof started: http://127.0.0.1:6060/debug/pprof")
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Errorf("pprof start failed: %v", err)
		}
	}()

	// 等待外部信号ctx被取消（Ctrl‑C / kill）
	<-ctx.Done()
	appLogger.Info("shutting down...")

	// 优雅关闭，设置10秒超时
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)
	appContainer.Shutdown(shutdownCtx)

	appLogger.Info("shutdown completed")
	return nil
}

package core

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// defaultLogger 是包级默认 SugaredLogger。
// 服务入口调用 InitLogger 初始化；未初始化时 Z() 回退到生产级 JSON 兜底 logger。
var defaultLogger *zap.SugaredLogger

// InitLogger 初始化包级默认 logger 并返回。
// level：debug/info/warn/error；format：console（开发友好）/ json（生产结构化，可被日志平台采集）。
func InitLogger(level, format string) (*zap.SugaredLogger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stdout"}
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	switch strings.ToLower(level) {
	case "debug":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "warn":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default: // info
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	if strings.ToLower(format) == "console" {
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	l, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	defaultLogger = l.Sugar()
	return defaultLogger, nil
}

// Z 返回包级默认 SugaredLogger。
// SugaredLogger 提供 Infof/Errorf/Warnf 等格式化方法，风格与标准库 log 一致，替换成本低；
// 需要携带结构化字段时用 With 派生（如 Z().With("user_id", uid).Infow("...")）。
func Z() *zap.SugaredLogger {
	if defaultLogger == nil {
		l, err := zap.NewProduction()
		if err != nil {
			// 极端兜底：zap 构造失败时退化为标准输出（理论不可达）
			panic("init fallback logger: " + err.Error())
		}
		defaultLogger = l.Sugar()
	}
	return defaultLogger
}

// SyncLogger 在进程退出前刷新缓冲日志（避免优雅关闭时丢失尾部日志）。
func SyncLogger() {
	if defaultLogger != nil {
		_ = defaultLogger.Sync()
	}
	_ = os.Stdout.Sync()
}

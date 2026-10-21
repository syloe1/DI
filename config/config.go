package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// App 应用总配置。
// 敏感字段（密码、密钥）在生产环境应通过环境变量注入，不要写入 yaml 提交 Git。
type App struct {
	Server   ServerConfig   `mapstructure:"server"`
	Mysql    MysqlConfig    `mapstructure:"mysql"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Jwt      JwtConfig      `mapstructure:"jwt"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Demo     DemoConfig     `mapstructure:"demo"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	Logger   LoggerConfig   `mapstructure:"logger"`
}

// =============================================================================
// 子配置结构体
// =============================================================================

type ServerConfig struct {
	Port         string        `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	CORSOrigins  []string      `mapstructure:"cors_origins"`
}

type MysqlConfig struct {
	Host            string        `mapstructure:"host"`
	Port            string        `mapstructure:"port"`
	Dbname          string        `mapstructure:"dbname"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	Db       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type JwtConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"` // 单位：小时（YAML 里写 24 = 24 小时）
}

// AuthConfig 注册/认证相关配置。
// RegisterInviteCode 非空时，注册接口必须携带相同的邀请码（演示/生产环境防批量注册）。
type AuthConfig struct {
	RegisterInviteCode string `mapstructure:"register_invite_code"`
}

// DemoConfig 演示账号配置：开启后前端登录页会出现「体验 Demo」入口，
// 可直接登录预置的只读演示账号（需先用 cmd/seed 写入种子数据）。
type DemoConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
}

type RabbitMQConfig struct {
	URL      string `mapstructure:"url"`
	Exchange string `mapstructure:"exchange"`
	Queue    string `mapstructure:"queue"`
}

// LoggerConfig 结构化日志配置。
// Level：debug/info/warn/error；Format：console（开发）/ json（生产）。
type LoggerConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// =============================================================================
// 便捷方法：连接串 / 地址拼装
// =============================================================================

// DSN 返回 MySQL 连接串，统一由配置负责格式，调用方无需知道内部拼法。
// multiStatements=true 是 golang-migrate 执行多语句迁移 SQL 所需；
// 业务 SQL 均走参数化/gorm 构造，不受影响。
func (c MysqlConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true",
		c.Username, c.Password, c.Host, c.Port, c.Dbname,
	)
}

// Addr 返回 Redis 地址 "host:port"。
func (c RedisConfig) Addr() string {
	return c.Host + ":" + c.Port
}

// =============================================================================
// 默认值
// =============================================================================

func (c *MysqlConfig) applyDefaults() {
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 10
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 100
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = time.Hour
	}
}

func (c *RedisConfig) applyDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 32
	}
}

func (c *ServerConfig) applyDefaults() {
	if c.Port == "" {
		c.Port = "19999"
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if len(c.CORSOrigins) == 0 {
		c.CORSOrigins = []string{"http://localhost:5173", "http://localhost:3000"}
	}
}

func (c *JwtConfig) applyDefaults() {
	if c.ExpireHours <= 0 {
		c.ExpireHours = 24
	}
}

func (c *DemoConfig) applyDefaults() {
	if c.Username == "" {
		c.Username = "demo"
	}
}

func (c *LoggerConfig) applyDefaults() {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Format == "" {
		c.Format = "console"
	}
}

// =============================================================================
// 加载
// =============================================================================

// Load 从 yaml 文件加载配置，并自动用环境变量覆盖。
// 环境变量前缀为 "APP"，如 APP_MYSQL_PASSWORD 会覆盖 mysql.password。
func Load(filePath string) (*App, error) {
	v := viper.New()
	v.SetConfigFile(filePath)

	// ---------- 环境变量覆盖 ----------
	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", filePath, err)
	}

	var cfg App
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validate: %w", err)
	}

	return &cfg, nil
}

// =============================================================================
// 默认值填充（内部）
// =============================================================================

func (c *App) applyDefaults() {
	c.Server.applyDefaults()
	c.Mysql.applyDefaults()
	c.Redis.applyDefaults()
	c.Jwt.applyDefaults()
	c.Demo.applyDefaults()
	c.Logger.applyDefaults()
}

// =============================================================================
// 验证
// =============================================================================

// Validate 检查必填配置项是否已填写。调用方应在服务启动前调用。
func (c *App) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server.port is required")
	}
	if c.Mysql.Host == "" {
		return fmt.Errorf("mysql.host is required")
	}
	if c.Mysql.Dbname == "" {
		return fmt.Errorf("mysql.dbname is required")
	}
	if c.Mysql.Username == "" {
		return fmt.Errorf("mysql.username is required")
	}
	if c.Mysql.Password == "" {
		return fmt.Errorf("mysql.password is required (set via env APP_MYSQL_PASSWORD)")
	}
	if c.Jwt.Secret == "" || c.Jwt.Secret == "change-me" {
		return fmt.Errorf("jwt.secret is required (set via env APP_JWT_SECRET)")
	}
	return nil
}

// =============================================================================
// 敏感信息脱敏（防止日志泄漏）
// =============================================================================

// GoString 实现 fmt.GoStringer，用 %#v 打印时密码不泄漏。
func (c MysqlConfig) GoString() string {
	return fmt.Sprintf("{Host:%q Port:%q Dbname:%q Username:%q Password:***}", c.Host, c.Port, c.Dbname, c.Username)
}

func (c RedisConfig) GoString() string {
	pwd := "***"
	if c.Password == "" {
		pwd = "(empty)"
	}
	return fmt.Sprintf("{Addr:%q DB:%d Password:%s}", c.Addr(), c.Db, pwd)
}

func (c JwtConfig) GoString() string {
	masked := "***"
	if len(c.Secret) > 4 {
		masked = c.Secret[:4] + "***"
	}
	return fmt.Sprintf("{Secret:%s ExpireHours:%v}", masked, c.ExpireHours)
}

func (c RabbitMQConfig) GoString() string {
	return fmt.Sprintf("{URL: *** Exchange:%q Queue:%q}", c.Exchange, c.Queue)
}

// =============================================================================
// Getter（兼容已有调用方）
// =============================================================================

func (c *App) GetMysqlConfig() MysqlConfig   { return c.Mysql }
func (c *App) GetRedisConfig() RedisConfig   { return c.Redis }
func (c *App) GetServerConfig() ServerConfig { return c.Server }
func (c *App) GetJwtConfig() JwtConfig       { return c.Jwt }
func (c *App) GetAuthConfig() AuthConfig     { return c.Auth }
func (c *App) GetDemoConfig() DemoConfig     { return c.Demo }
func (c *App) GetCORSOrigins() []string      { return c.Server.CORSOrigins }
func (c *App) GetLoggerConfig() LoggerConfig { return c.Logger }

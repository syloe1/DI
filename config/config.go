package config

import (
	"log"

	"github.com/spf13/viper"
)

// 抽象层，便于mock测试
type ConfigProvider interface {
	GetMysqlConfig() MysqlConfig
	GetRedisConfig() RedisConfig
	GetServerConfig() ServerConfigItem
	GetJwtConfig() JwtConfig
}

func (c *ServerConfig) GetMysqlConfig() MysqlConfig {
	return c.Mysql
}
func (c *ServerConfig) GetRedisConfig() RedisConfig {
	return c.Redis
}
func (c *ServerConfig) GetServerConfig() ServerConfigItem {
	return c.Server
}
func (c *ServerConfig) GetJwtConfig() JwtConfig {
	return c.Jwt
}

type ServerConfig struct {
	Mysql  MysqlConfig
	Server ServerConfigItem
	Redis  RedisConfig
	Jwt    JwtConfig
}
type ServerConfigItem struct {
	Port string
}
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	Db       int
}
type MysqlConfig struct {
	Host     string
	Port     string
	Dbname   string
	Username string
	Password string
}
type JwtConfig struct {
	Secret string
}

func InitConfig(filePath string) (ConfigProvider, error) {
	viper.SetConfigFile(filePath)
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("读取配置文件失败: %v", err)
		return nil, err
	}
	var cfg ServerConfig
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Printf("解析配置文件失败: %v", err)
		return nil, err
	}
	log.Println("✅ 配置文件加载成功")
	return &cfg, nil
}

var GlobalConfig ConfigProvider

func InitGlobalConfig() {
	cfg, err := InitConfig("config/config.yaml")
	if err != nil {
		log.Fatal("初始化配置失败:", err)
	}
	GlobalConfig = cfg
}

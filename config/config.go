package config

import "github.com/spf13/viper"

type App struct {
	Mysql  MysqlConfig      `mapstructure:"mysql"`
	Server ServerConfigItem `mapstructure:"server"`
	Redis  RedisConfig      `mapstructure:"redis"`
	Jwt    JwtConfig        `mapstructure:"jwt"`
}

type ConfigProvider interface {
	GetMysqlConfig() MysqlConfig
	GetRedisConfig() RedisConfig
	GetServerConfig() ServerConfigItem
	GetJwtConfig() JwtConfig
}

type ServerConfigItem struct {
	Port string `mapstructure:"port"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Password string `mapstructure:"password"`
	Db       int    `mapstructure:"db"`
}

type MysqlConfig struct {
	Host     string `mapstructure:"host"`
	Port     string `mapstructure:"port"`
	Dbname   string `mapstructure:"dbname"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type JwtConfig struct {
	Secret string `mapstructure:"secret"`
}

func (c *App) GetMysqlConfig() MysqlConfig {
	return c.Mysql
}

func (c *App) GetRedisConfig() RedisConfig {
	return c.Redis
}

func (c *App) GetServerConfig() ServerConfigItem {
	return c.Server
}

func (c *App) GetJwtConfig() JwtConfig {
	return c.Jwt
}

func Load(filePath string) (*App, error) {
	v := viper.New()
	v.SetConfigFile(filePath)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg App
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

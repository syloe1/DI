package core

import (
	"go-admin/config"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitMysql(cfg *config.ServerConfig) (*gorm.DB, error) {
	dsn := cfg.Mysql.Username + ":" + cfg.Mysql.Password +
		"@tcp(" + cfg.Mysql.Host + ":" + cfg.Mysql.Port + ")/" +
		cfg.Mysql.Dbname + "?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	log.Println("✅ MySQL连接成功")

	return db, nil
}

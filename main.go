package main

import (
	"go-admin/config"
	"go-admin/core"
	"go-admin/internal/container"
	"go-admin/model"
	"go-admin/router"
)

func main() {
	// 初始化配置
	config.InitGlobalConfig()

	// 初始化数据库和Redis
	db, err := core.InitMysql(config.GlobalConfig.(*config.ServerConfig))
	if err != nil {
		panic("数据库初始化失败: " + err.Error())
	}

	core.InitRedis()

	// 自动迁移数据库表
	if db != nil {
		db.AutoMigrate(&model.User{})
		db.AutoMigrate(&model.Post{})
		db.AutoMigrate(&model.Comment{})
		db.AutoMigrate(&model.Like{})
		db.AutoMigrate(&model.Dislike{})
		db.AutoMigrate(&model.Collect{})
		db.AutoMigrate(&model.Share{})
		db.AutoMigrate(&model.UserRelation{})
		db.AutoMigrate(&model.Message{})
	}

	// 创建依赖注入容器
	container := container.NewContainer(db, core.RDB, []byte(config.GlobalConfig.GetJwtConfig().Secret))

	// 使用依赖注入版本的路由
	r := router.InitDependencyInjectionRouter(container)
	r.Run(":" + config.GlobalConfig.GetServerConfig().Port)
}

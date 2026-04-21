package core

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.Post{},
		&model.Comment{},
		&model.Like{},
		&model.Dislike{},
		&model.Collect{},
		&model.Share{},
		&model.UserRelation{},
		&model.Message{},
	)
}

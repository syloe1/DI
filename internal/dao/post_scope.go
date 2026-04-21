package dao

import (
	"go-admin/internal/domain/model"
	"strings"

	"gorm.io/gorm"
)

func WithPublishedPost() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ? AND is_public = true", model.PostStatusPublished)
	}
}

func WithTopic(topic string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if strings.TrimSpace(topic) == "" {
			return db
		}
		return db.Where("topics LIKE ?", "%"+strings.TrimSpace(topic)+"%")
	}
}

func WithPagination(page int, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 10
		}
		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

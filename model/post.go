package model

import (
	"time"

	"gorm.io/gorm"
)

// 帖子状态
const (
	PostStatusPublished = "published" //已发布
	PostStatusDraft     = "draft"     //草稿
	PostStatusScheduled = "scheduled" //定时
)

type Post struct {
	gorm.Model
	Title     string     `gorm:"size:128;not null" json:"title"`
	Content   string     `gorm:"type:text" json:"content"`
	IsPublic  bool       `json:"is_public"`
	Status    string     `gorm:"size:16;default:published" json:"status"`
	PublishAt *time.Time `json:"publish_at"`
	Topics    string     `gorm:"size:255" json:"topics"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	Username  string     `gorm:"size:32" json:"username"`
	Images    string     `gorm:"size:512" json:"images"`
}

func (Post) TableName() string {
	return "posts"
}

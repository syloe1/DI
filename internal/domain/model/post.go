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
	Title     string     `gorm:"size:128;not null" json:"title" binding:"required,min=1,max=128`
	Content   string     `gorm:"type:text" json:"content" binding:"required,min=1,max=5000"`
	IsPublic  bool       `json:"is_public"`
	Status    string     `gorm:"size:16;default:published" json:"status" binding:"omitempty,oneof=published draft scheduled"`
	PublishAt *time.Time `json:"publish_at"`
	Topics    string     `gorm:"size:255" json:"topics" binding:"omitempty,max=255"`
	UserID    uint       `gorm:"not null;index" json:"user_id"`
	Username  string     `gorm:"size:32" json:"username"`
	User      User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Images    string     `gorm:"size:512" json:"images" binding:"omitempty,max=512"`
	ViewCount int64      `gorm:"-" json:"view_count"`
}

func (Post) TableName() string {
	return "posts"
}

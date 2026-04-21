package model

import "gorm.io/gorm"

type Like struct {
	gorm.Model
	UserID uint `gorm:"not null;index:idx_user_post" json:"user_id"`
	PostID uint `gorm:"not null;index:idx_user_post" json:"post_id"`
}

func (Like) TableName() string {
	return "likes"
}

type Dislike struct {
	gorm.Model
	UserID uint `gorm:"not null;index:idx_user_post" json:"user_id"`
	PostID uint `gorm:"not null;index:idx_user_post" json:"post_id"`
}

func (Dislike) TableName() string {
	return "dislikes"
}

type Collect struct {
	gorm.Model
	UserID uint   `gorm:"not null;index:idx_user_post" json:"user_id"`
	PostID uint   `gorm:"not null;index:idx_user_post" json:"post_id"`
	Remark string `gorm:"size:128;default:''" json:"remark"`
}

func (Collect) TableName() string {
	return "collects"
}

type Share struct {
	gorm.Model
	UserID   uint   `gorm:"not null;index" json:"user_id"`
	PostID   uint   `gorm:"not null;index" json:"post_id"`
	Platform string `gorm:"size:32;not null" json:"platform"`
	ShareURL string `gorm:"size:255;not null" json:"share_url"`
}

func (Share) TableName() string {
	return "shares"
}

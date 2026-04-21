package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	CommentStatusNormal  = "normal"
	CommentStatusDeleted = "deleted"
)

type Comment struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	PostID    uint           `gorm:"not null" json:"post_id"`
	UserID    uint           `gorm:"not null" json:"user_id"`
	Username  string         `gorm:"size:32;not null" json:"username"`
	User      User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Content   string         `gorm:"type:text;not null" json:"content"`
	ParentId  uint           `gorm:"default:0" json:"parent_id"`
	Status    string         `gorm:"default:'normal'" json:"status"`
}

func (Comment) TableName() string {
	return "comments"
}

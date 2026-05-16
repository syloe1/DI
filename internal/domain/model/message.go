package model

import "gorm.io/gorm"

type Message struct {
	gorm.Model
	FromUID uint   `gorm:"not null;index:idx_from_to" json:"from_uid"`
	ToUID   uint   `gorm:"not null;index:idx_from_to" json:"to_uid"`
	Content string `gorm:"type:text;not null" json:"content"`
	IsRead  bool   `gorm:"default:false" json:"is_read"`
}
type ChatGroupMessage struct {
	gorm.Model
	GroupID   uint   `gorm:"not null;index" json:"group_id"`
	SenderUID uint   `gorm:"not null;index" json:"sender_uid"`
	Content   string `gorm:"type:text;not null" json:"content"`
}

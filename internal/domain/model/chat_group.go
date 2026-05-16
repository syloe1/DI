package model

import (
	"gorm.io/gorm"
)

const (
	ChatGroupStatusNormal    = "normal"
	ChatGroupStatusDissolved = "dissolved"
	ChatGroupStatusBanned    = "banned"
)

type ChatGroup struct {
	gorm.Model
	Name     string `gorm:"type:varchar(100);not null" json:"name"`
	OwnerUID uint   `gorm:"not null" json:"owner_uid"`
	Status   string `gorm:"type:varchar(20);not null;default:'normal';index" json:"status"`
}

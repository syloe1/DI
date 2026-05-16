package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ChatGroupMemberRoleOwner    = "owner"
	ChatGroupMemberRoleAdmin    = "admin"
	ChatGroupMemberRoleMember   = "member"
	ChatGroupMemberStatusActive = "active"
	ChatGroupMemberStatusLeft   = "left"
	ChatGroupMemberStatusKicked = "kicked"
)

type ChatGroupMember struct {
	gorm.Model
	GroupID  uint       `gorm:"not null;index:idx_group_user,unique" json:"group_id"`
	UserID   uint       `gorm:"not null;index:idx_group_user,unique;index" json:"user_id"`
	Role     string     `gorm:"type:varchar(20);not null;default:'member';index" json:"role"`
	Status   string     `gorm:"type:varchar(20);not null;default:'active';index" json:"status"`
	JoinedAt time.Time  `gorm:"not null" json:"joined_at"`
	LeftAt   *time.Time `json:"left_at,omitempty"`
}

package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ChatGroupJoinRequestStatusPending  = "pending"
	ChatGroupJoinRequestStatusApproved = "approved"
	ChatGroupJoinRequestStatusRejected = "rejected"
	ChatGroupJoinRequestStatusExpired  = "expired"
)

type ChatGroupJoinRequest struct {
	gorm.Model
	GroupID     uint       `gorm:"not null;index" json:"group_id"`
	UserID      uint       `gorm:"not null;index" json:"user_id"`
	Reason      string     `gorm:"type:varchar(255)" json:"reason"`
	Status      string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	ReviewerUID *uint      `gorm:"index" json:"reviewer_uid,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	ExpiredAt   time.Time  `gorm:"not null;index" json:"expired_at"`
}

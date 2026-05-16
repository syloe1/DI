package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ChatGroupInvitationStatusPending  = "pending"
	ChatGroupInvitationStatusAccepted = "accepted"
	ChatGroupInvitationStatusRejected = "rejected"
	ChatGroupInvitationStatusExpired  = "expired"
)

type ChatGroupInvitation struct {
	gorm.Model
	GroupID    uint       `gorm:"not null;index" json:"group_id"`
	InviterUID uint       `gorm:"not null;index" json:"inviter_uid"`
	InviteeUID uint       `gorm:"not null;index" json:"invitee_uid"`
	Status     string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	ExpiredAt  time.Time  `gorm:"not null;index" json:"expired_at"`
	HandledAt  *time.Time `json:"handled_at,omitempty"`
}

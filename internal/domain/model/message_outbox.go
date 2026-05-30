package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	MessageOutboxStatusPending    = "pending"
	MessageOutboxStatusProcessing = "processing"
	MessageOutboxStatusPublished  = "published"
	MessageOutboxStatusFailed     = "failed"
)

type MessageOutbox struct {
	gorm.Model
	EventType   string     `gorm:"type:varchar(64);not null;index:idx_outbox_status_retry" json:"event_type"`
	AggregateID string     `gorm:"type:varchar(64);not null;index" json:"aggregate_id"`
	Payload     string     `gorm:"type:json;not null" json:"payload"`
	Status      string     `gorm:"type:varchar(20);not null;default:'pending';index:idx_outbox_status_retry" json:"status"`
	RetryCount  int        `gorm:"not null;default:0" json:"retry_count"`
	NextRetryAt time.Time  `gorm:"not null;index:idx_outbox_status_retry" json:"next_retry_at"`
	ClaimedAt   *time.Time `gorm:"index:idx_outbox_status_retry" json:"claimed_at,omitempty"`
	ClaimedBy   string     `gorm:"type:varchar(128);index" json:"claimed_by,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
}

package dto

import "time"

type GroupMessageCreatedEvent struct {
	Type      string    `json:"type"`
	MessageID uint      `json:"message_id"`
	GroupID   uint      `json:"group_id"`
	FromUID   uint      `json:"from_uid"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

package dto

import "time"

type GroupMessageCreatedEvent struct {
	Type      string    `json:"type"`
	RequestID string    `json:"request_id,omitempty"`
	MessageID uint      `json:"message_id"`
	GroupID   uint      `json:"group_id"`
	FromUID   uint      `json:"from_uid"`
	Username  string    `json:"username,omitempty"` // 发送者用户名：HTTP/WS 两条链路都写入，推送直接可用
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

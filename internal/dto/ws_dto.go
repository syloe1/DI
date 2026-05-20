package dto

type WSInboundMessage struct {
	Type    string `json:"type"`
	ToUID   uint   `json:"to_uid"`
	GroupID uint   `json:"group_id"`
	Content string `json:"content"`
}

type WSOutboundMessage struct {
	Type      string `json:"type"`
	MessageID uint   `json:"message_id,omitempty"`
	GroupID   uint   `json:"group_id"`
	FromUID   uint   `json:"from_uid,omitempty"`
	ToUID     uint   `json:"to_uid,omitempty"`
	Content   string `json:"content,omitempty"`
	Time      string `json:"time,omitempty"`
	Message   string `json:"message,omitempty"`
}

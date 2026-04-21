package dto

type MessageListQuery struct {
	PeerID   uint `form:"peer_id" binding:"required,min=1"`
	Page     int  `form:"page" binding:"omitempty,min=1"`
	PageSize int  `form:"page_size" binding:"omitempty,min=1,max=100"`
}

type SendMessageRequest struct {
	ToUID   uint   `json:"to_uid" binding:"required"`
	Content string `json:"content" binding:"required"`
}

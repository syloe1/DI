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

// MessageSyncQuery 断线重连后的增量同步参数：拉取 id 大于 AfterID 的消息
type MessageSyncQuery struct {
	AfterID uint `form:"after_id" binding:"omitempty,min=1"`
	Limit   int  `form:"limit" binding:"omitempty,min=1,max=100"`
}

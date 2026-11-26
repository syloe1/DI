package handler

import (
	"strconv"

	"di/internal/dto"
	"di/internal/service"
	"di/internal/transport"
	"github.com/gin-gonic/gin"
)

type MessageHandler struct {
	svc *service.MessageService
}

func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

func (h *MessageHandler) GetConversations(c *gin.Context) {
	cursor, _ := strconv.ParseUint(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, nextCursor, err := h.svc.GetConversations(c.Request.Context(), c.GetUint("userID"), uint(cursor), limit)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, gin.H{"items": items, "next_cursor": nextCursor})
}

func (h *MessageHandler) GetMessageList(c *gin.Context) {
	var query dto.MessageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.GetMessageList(c.Request.Context(), c.GetUint("userID"), query.PeerID, query.Page, query.PageSize)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

// SyncMessages 断线重连后的增量同步：GET /auth/message/sync?after_id=&limit=
func (h *MessageHandler) SyncMessages(c *gin.Context) {
	var query dto.MessageSyncQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.SyncMessages(c.Request.Context(), c.GetUint("userID"), query.AfterID, query.Limit)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.SendMessage(c.Request.Context(), c.GetUint("userID"), req.ToUID, req.Content)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	if err := h.svc.DeleteMessage(c.Request.Context(), c.GetUint("userID"), c.Param("id")); err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.SuccessWithMessage(c, "delete message success", nil)
}

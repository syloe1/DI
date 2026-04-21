package handler

import (
	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type MessageHandler struct {
	svc *service.MessageService
}

func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

func (h *MessageHandler) GetConversations(c *gin.Context) {
	data, err := h.svc.GetConversations(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *MessageHandler) GetMessageList(c *gin.Context) {
	var query dto.MessageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.GetMessageList(c.GetUint("userID"), query.PeerID, query.Page, query.PageSize)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *MessageHandler) SendMessage(c *gin.Context) {
	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.SendMessage(c.GetUint("userID"), req.ToUID, req.Content)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	if err := h.svc.DeleteMessage(c.GetUint("userID"), c.Param("id")); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "delete message success", nil)
}

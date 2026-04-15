package api

import (
	"fmt"
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// MessageAPI 封装消息相关的处理器，持有依赖
type MessageAPI struct {
	messageRepo repository.MessageRepository // 注入的Repository接口
}

// NewMessageAPI 构造函数：注入MessageRepository依赖
func NewMessageAPI(messageRepo repository.MessageRepository) *MessageAPI {
	return &MessageAPI{messageRepo: messageRepo}
}

// GetConversations 获取会话列表
func (api *MessageAPI) GetConversations(c *gin.Context) {
	userID := c.GetUint("userID")

	// 使用依赖注入的Repository获取会话列表
	conversations, err := api.messageRepo.GetConversations(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取会话列表失败")
		return
	}

	// 构建响应数据
	result := make([]gin.H, 0)
	for _, conv := range conversations {
		// 获取对应用户信息
		user, err := api.messageRepo.FindUserByID(conv.PeerUID)
		if err != nil {
			continue
		}

		// 计算未读消息数
		unread, _ := api.messageRepo.CountUnreadMessages(userID, conv.PeerUID)

		result = append(result, gin.H{
			"uid":       conv.PeerUID,
			"username":  user.Username,
			"avatar":    "", // User模型中没有Avatar字段
			"last_msg":  conv.LastMsg,
			"last_time": conv.LastTime,
			"unread":    unread,
		})
	}

	core.Success(c, result)
}

// GetMessageList 获取消息列表
func (api *MessageAPI) GetMessageList(c *gin.Context) {
	userID := c.GetUint("userID")
	peerIDStr := c.Query("peer_id")

	if peerIDStr == "" {
		core.Fail(c, http.StatusBadRequest, "peer_id参数不能为空")
		return
	}

	peerID, err := strconv.ParseUint(peerIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的peer_id")
		return
	}

	// 检查对方用户是否存在
	peerUser, err := api.messageRepo.FindUserByID(uint(peerID))
	if err != nil {
		core.Fail(c, http.StatusNotFound, "对方用户不存在")
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// 使用依赖注入的Repository获取消息列表
	messages, err := api.messageRepo.GetMessages(userID, uint(peerID), limit)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取消息列表失败")
		return
	}

	// 标记消息为已读
	if err := api.messageRepo.MarkMessagesAsRead(userID, uint(peerID)); err != nil {
		// 标记已读失败不影响消息获取，只记录日志
		fmt.Printf("标记消息已读失败: %v\n", err)
	}

	// 构建响应数据
	result := make([]gin.H, 0)
	for _, msg := range messages {
		result = append(result, gin.H{
			"id":         msg.ID,
			"from_uid":   msg.FromUID,
			"to_uid":     msg.ToUID,
			"content":    msg.Content,
			"is_read":    msg.IsRead,
			"created_at": msg.CreatedAt,
		})
	}

	core.Success(c, gin.H{
		"peer_user": gin.H{
			"id":       peerUser.ID,
			"username": peerUser.Username,
			"avatar":   "", // User模型中没有Avatar字段
		},
		"messages": result,
	})
}

// SendMessage 发送消息
func (api *MessageAPI) SendMessage(c *gin.Context) {
	userID := c.GetUint("userID")

	var req struct {
		ToUID   uint   `json:"to_uid" binding:"required"`
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 检查对方用户是否存在
	toUser, err := api.messageRepo.FindUserByID(req.ToUID)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "对方用户不存在")
		return
	}

	// 不能给自己发消息
	if userID == req.ToUID {
		core.Fail(c, http.StatusBadRequest, "不能给自己发消息")
		return
	}

	// 创建消息
	message := &model.Message{
		FromUID:  userID,
		ToUID:    req.ToUID,
		Content:  req.Content,
		IsRead:   false,
	}

	// 使用依赖注入的Repository创建消息
	if err := api.messageRepo.CreateMessage(message); err != nil {
		core.Fail(c, http.StatusInternalServerError, "发送消息失败")
		return
	}

	core.Success(c, gin.H{
		"message_id": message.ID,
		"to_user": gin.H{
			"id":       toUser.ID,
			"username": toUser.Username,
		},
		"content":    req.Content,
		"created_at": message.CreatedAt,
	})
}

// DeleteMessage 删除消息
func (api *MessageAPI) DeleteMessage(c *gin.Context) {
	userID := c.GetUint("userID")
	messageIDStr := c.Param("id")

	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的消息ID")
		return
	}

	// 使用依赖注入的Repository查询消息
	message, err := api.messageRepo.FindMessageByID(uint(messageID))
	if err != nil {
		core.Fail(c, http.StatusNotFound, "消息不存在")
		return
	}

	// 检查权限：只能删除自己发送的消息
	if message.FromUID != userID {
		core.Fail(c, http.StatusForbidden, "只能删除自己发送的消息")
		return
	}

	// 使用依赖注入的Repository删除消息
	if err := api.messageRepo.DeleteMessage(message); err != nil {
		core.Fail(c, http.StatusInternalServerError, "删除消息失败")
		return
	}

	core.Success(c, "删除消息成功")
}

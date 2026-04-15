package api

import (
	"go-admin/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 以下方法需要实现，暂时提供占位实现

// HandleWebSocket WebSocket处理（需要实现）
func (s *UserService) HandleWebSocket(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "WebSocket功能暂未实现")
}

// GetInteractCount 获取互动统计（需要实现）
func (s *UserService) GetInteractCount(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "互动统计功能暂未实现")
}

// ToggleLike 点赞/取消点赞（需要实现）
func (s *UserService) ToggleLike(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "点赞功能暂未实现")
}

// ToggleDislike 点踩/取消点踩（需要实现）
func (s *UserService) ToggleDislike(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "点踩功能暂未实现")
}

// ToggleCollect 收藏/取消收藏（需要实现）
func (s *UserService) ToggleCollect(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "收藏功能暂未实现")
}

// Share 分享帖子（需要实现）
func (s *UserService) Share(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "分享功能暂未实现")
}

// GetInteractStatus 获取互动状态（需要实现）
func (s *UserService) GetInteractStatus(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "互动状态功能暂未实现")
}

// FollowUser 关注/取消关注用户（需要实现）
func (s *UserService) FollowUser(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "关注功能暂未实现")
}

// BlockUser 拉黑/取消拉黑用户（需要实现）
func (s *UserService) BlockUser(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "拉黑功能暂未实现")
}

// GetRelationStatus 获取关系状态（需要实现）
func (s *UserService) GetRelationStatus(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "关系状态功能暂未实现")
}

// GetFollowList 获取关注列表（需要实现）
func (s *UserService) GetFollowList(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "关注列表功能暂未实现")
}

// GetFollowerList 获取粉丝列表（需要实现）
func (s *UserService) GetFollowerList(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "粉丝列表功能暂未实现")
}

// GetBlockList 获取拉黑列表（需要实现）
func (s *UserService) GetBlockList(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "拉黑列表功能暂未实现")
}

// GetConversations 获取会话列表（需要实现）
func (s *UserService) GetConversations(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "会话列表功能暂未实现")
}

// GetMessageList 获取消息列表（需要实现）
func (s *UserService) GetMessageList(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "消息列表功能暂未实现")
}

// SendMessage 发送消息（需要实现）
func (s *UserService) SendMessage(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "发送消息功能暂未实现")
}
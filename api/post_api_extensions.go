package api

import (
	"go-admin/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 以下方法需要实现，暂时提供占位实现

// GetInteractCount 获取互动统计（需要实现）
func (api *PostAPI) GetInteractCount(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "互动统计功能暂未实现")
}

// ToggleLike 点赞/取消点赞（需要实现）
func (api *PostAPI) ToggleLike(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "点赞功能暂未实现")
}

// ToggleDislike 点踩/取消点踩（需要实现）
func (api *PostAPI) ToggleDislike(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "点踩功能暂未实现")
}

// ToggleCollect 收藏/取消收藏（需要实现）
func (api *PostAPI) ToggleCollect(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "收藏功能暂未实现")
}

// Share 分享帖子（需要实现）
func (api *PostAPI) Share(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "分享功能暂未实现")
}

// GetInteractStatus 获取互动状态（需要实现）
func (api *PostAPI) GetInteractStatus(c *gin.Context) {
	core.Fail(c, http.StatusNotImplemented, "互动状态功能暂未实现")
}
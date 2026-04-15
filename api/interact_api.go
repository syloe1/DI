package api

import (
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// InteractAPI 封装互动相关的处理器，持有依赖
type InteractAPI struct {
	interactRepo repository.InteractRepository // 注入的Repository接口
}

// NewInteractAPI 构造函数：注入InteractRepository依赖
func NewInteractAPI(interactRepo repository.InteractRepository) *InteractAPI {
	return &InteractAPI{interactRepo: interactRepo}
}

// ToggleLike 点赞/取消点赞
func (api *InteractAPI) ToggleLike(c *gin.Context) {
	userID := c.GetUint("userID")
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 使用依赖注入的Repository查询点赞状态
	like, err := api.interactRepo.FindLike(userID, uint(postID))

	if err != nil {
		// 没点赞 → 点赞
		// 先检查是否已点踩，如果已点踩则取消点踩（点赞和点踩互斥）
		dislike, _ := api.interactRepo.FindDislike(userID, uint(postID))
		if dislike != nil {
			api.interactRepo.DeleteDislike(dislike)
		}

		// 创建点赞
		newLike := &model.Like{
			UserID: userID,
			PostID: uint(postID),
		}
		if err := api.interactRepo.CreateLike(newLike); err != nil {
			core.Fail(c, http.StatusInternalServerError, "点赞失败")
			return
		}

		core.Success(c, gin.H{"status": true, "message": "点赞成功"})
		return
	}

	// 已点赞 → 取消点赞
	if err := api.interactRepo.DeleteLike(like); err != nil {
		core.Fail(c, http.StatusInternalServerError, "取消点赞失败")
		return
	}

	core.Success(c, gin.H{"status": false, "message": "取消点赞"})
}

// ToggleDislike 点踩/取消点踩
func (api *InteractAPI) ToggleDislike(c *gin.Context) {
	userID := c.GetUint("userID")
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 使用依赖注入的Repository查询点踩状态
	dislike, err := api.interactRepo.FindDislike(userID, uint(postID))

	if err != nil {
		// 没点踩 → 点踩
		// 先检查是否已点赞，如果已点赞则取消点赞（点赞和点踩互斥）
		like, _ := api.interactRepo.FindLike(userID, uint(postID))
		if like != nil {
			api.interactRepo.DeleteLike(like)
		}

		// 创建点踩
		newDislike := &model.Dislike{
			UserID: userID,
			PostID: uint(postID),
		}
		if err := api.interactRepo.CreateDislike(newDislike); err != nil {
			core.Fail(c, http.StatusInternalServerError, "点踩失败")
			return
		}

		core.Success(c, gin.H{"status": true, "message": "点踩成功"})
		return
	}

	// 已点踩 → 取消点踩
	if err := api.interactRepo.DeleteDislike(dislike); err != nil {
		core.Fail(c, http.StatusInternalServerError, "取消点踩失败")
		return
	}

	core.Success(c, gin.H{"status": false, "message": "取消点踩"})
}

// ToggleCollect 收藏/取消收藏
func (api *InteractAPI) ToggleCollect(c *gin.Context) {
	userID := c.GetUint("userID")
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 使用依赖注入的Repository查询收藏状态
	collect, err := api.interactRepo.FindCollect(userID, uint(postID))

	if err != nil {
		// 没收藏 → 收藏
		newCollect := &model.Collect{
			UserID: userID,
			PostID: uint(postID),
		}
		if err := api.interactRepo.CreateCollect(newCollect); err != nil {
			core.Fail(c, http.StatusInternalServerError, "收藏失败")
			return
		}

		core.Success(c, gin.H{"status": true, "message": "收藏成功"})
		return
	}

	// 已收藏 → 取消收藏
	if err := api.interactRepo.DeleteCollect(collect); err != nil {
		core.Fail(c, http.StatusInternalServerError, "取消收藏失败")
		return
	}

	core.Success(c, gin.H{"status": false, "message": "取消收藏"})
}

// Share 分享帖子
func (api *InteractAPI) Share(c *gin.Context) {
	userID := c.GetUint("userID")
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 创建分享记录
	share := &model.Share{
		UserID: userID,
		PostID: uint(postID),
	}

	if err := api.interactRepo.CreateShare(share); err != nil {
		core.Fail(c, http.StatusInternalServerError, "分享失败")
		return
	}

	core.Success(c, gin.H{"message": "分享成功"})
}

// GetInteractStatus 获取互动状态
func (api *InteractAPI) GetInteractStatus(c *gin.Context) {
	userID := c.GetUint("userID")
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 使用依赖注入的Repository获取互动状态
	status, err := api.interactRepo.GetInteractStatus(userID, uint(postID))
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取互动状态失败")
		return
	}

	core.Success(c, status)
}

// GetInteractCount 获取互动统计
func (api *InteractAPI) GetInteractCount(c *gin.Context) {
	postIDStr := c.Param("post_id")

	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "无效的帖子ID")
		return
	}

	// 使用依赖注入的Repository获取互动统计
	counts, err := api.interactRepo.GetInteractCount(uint(postID))
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取互动统计失败")
		return
	}

	core.Success(c, counts)
}

package api

import (
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	liked := false
	err = api.interactRepo.Transaction(func(repoWithTx repository.InteractRepository) error {
		like, err := repoWithTx.FindLike(userID, uint(postID))
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			dislike, err := repoWithTx.FindDislike(userID, uint(postID))
			if err == nil && dislike != nil {
				if err := repoWithTx.DeleteDislike(dislike); err != nil {
					return err
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}

			newLike := &model.Like{
				UserID: userID,
				PostID: uint(postID),
			}
			if err := repoWithTx.CreateLike(newLike); err != nil {
				return err
			}

			liked = true
			return nil
		}

		if err := repoWithTx.DeleteLike(like); err != nil {
			return err
		}

		liked = false
		return nil
	})
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "点赞失败")
		return
	}

	if liked {
		core.Success(c, gin.H{"status": true, "message": "点赞成功"})
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

	disliked := false
	err = api.interactRepo.Transaction(func(repoWithTx repository.InteractRepository) error {
		dislike, err := repoWithTx.FindDislike(userID, uint(postID))
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			like, err := repoWithTx.FindLike(userID, uint(postID))
			if err == nil && like != nil {
				if err := repoWithTx.DeleteLike(like); err != nil {
					return err
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}

			newDislike := &model.Dislike{
				UserID: userID,
				PostID: uint(postID),
			}
			if err := repoWithTx.CreateDislike(newDislike); err != nil {
				return err
			}
			disliked = true
			return nil
		}
		if err := repoWithTx.DeleteDislike(dislike); err != nil {
			return err
		}
		disliked = false
		return nil
	})
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "点踩失败")
		return
	}
	if disliked {
		core.Success(c, gin.H{"status": true, "message": "点踩成功"})
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

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

// CommentAPI 封装评论相关的处理器，持有依赖
type CommentAPI struct {
	commentRepo repository.CommentRepository // 注入的Repository接口
}

// NewCommentAPI 构造函数：注入CommentRepository依赖
func NewCommentAPI(commentRepo repository.CommentRepository) *CommentAPI {
	return &CommentAPI{commentRepo: commentRepo}
}

// CreateComment 创建评论
func (api *CommentAPI) CreateComment(c *gin.Context) {
	var req struct {
		PostID   uint   `json:"post_id" binding:"required"`
		Content  string `json:"content" binding:"required"`
		ParentID uint   `json:"parent_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 检查帖子是否存在
	_, err := api.commentRepo.FindPostByID(req.PostID)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "帖子不存在")
		return
	}

	// 构建评论对象
	comment := model.Comment{
		PostID:   req.PostID,
		UserID:   c.GetUint("userID"),
		Username: c.GetString("username"),
		Content:  req.Content,
		ParentId: req.ParentID,
		Status:   model.CommentStatusNormal,
	}

	// 使用依赖注入的Repository创建评论
	if err := api.commentRepo.Create(&comment); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建评论失败")
		return
	}

	core.Success(c, comment)
}

// DeleteComment 删除评论
func (api *CommentAPI) DeleteComment(c *gin.Context) {
	commentID := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentUserRole := c.GetString("role")

	// 使用依赖注入的Repository查询评论
	comment, err := api.commentRepo.FindByID(commentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			core.Fail(c, http.StatusNotFound, "评论不存在")
		} else {
			core.Fail(c, http.StatusInternalServerError, "查询评论失败")
		}
		return
	}

	// 检查权限：
	// 1. 评论作者可以删除自己的评论
	// 2. 超级管理员可以删除任何评论
	// 3. 管理员可以删除普通用户的评论，但不能删除其他管理员或超级管理员的评论
	if comment.UserID != currentUserID {
		if currentUserRole == "superadmin" {
			// 超级管理员可以删除任何评论
		} else if currentUserRole == "admin" {
			// 管理员只能删除普通用户的评论
			commentUser, err := api.commentRepo.FindUserByID(comment.UserID)
			if err == nil && commentUser.Role != "user" {
				core.Fail(c, http.StatusForbidden, "管理员只能删除普通用户的评论")
				return
			}
		} else {
			// 普通用户只能删除自己的评论
			core.Fail(c, http.StatusForbidden, "无权删除此评论")
			return
		}
	}

	// 使用依赖注入的Repository删除评论
	if err := api.commentRepo.Delete(comment); err != nil {
		core.Fail(c, http.StatusInternalServerError, "删除评论失败")
		return
	}

	core.Success(c, "删除评论成功")
}

// UpdateComment 更新评论
func (api *CommentAPI) UpdateComment(c *gin.Context) {
	commentID := c.Param("id")
	currentUserID := c.GetUint("userID")

	// 使用依赖注入的Repository查询评论
	comment, err := api.commentRepo.FindByID(commentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			core.Fail(c, http.StatusNotFound, "评论不存在")
		} else {
			core.Fail(c, http.StatusInternalServerError, "查询评论失败")
		}
		return
	}

	// 检查权限：只能修改自己的评论
	if comment.UserID != currentUserID {
		core.Fail(c, http.StatusForbidden, "无权修改他人评论")
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 使用依赖注入的Repository更新评论
	updates := map[string]interface{}{
		"content": req.Content,
	}

	if err := api.commentRepo.Update(comment, updates); err != nil {
		core.Fail(c, http.StatusInternalServerError, "更新评论失败")
		return
	}

	core.Success(c, "更新评论成功")
}

// GetPostComments 获取帖子评论
func (api *CommentAPI) GetPostComments(c *gin.Context) {
	postID := c.Param("post_id")

	// 使用依赖注入的Repository查询评论
	comments, err := api.commentRepo.FindByPostID(postID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取评论失败")
		return
	}

	core.Success(c, comments)
}

// GetUserComments 获取用户评论（需要认证）
func (api *CommentAPI) GetUserComments(c *gin.Context) {
	userID := c.GetUint("userID")

	// 使用依赖注入的Repository查询评论
	comments, err := api.commentRepo.FindByUserID(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取用户评论失败")
		return
	}

	core.Success(c, comments)
}

// GetPublicUserComments 获取公开用户评论（无需认证）
func (api *CommentAPI) GetPublicUserComments(c *gin.Context) {
	userIDStr := c.Param("id")

	// 转换用户ID
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	// 使用依赖注入的Repository查询评论
	comments, err := api.commentRepo.FindByUserID(uint(userID))
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取用户评论失败")
		return
	}

	core.Success(c, comments)
}

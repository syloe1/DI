package handler

import (
	"net/http"
	"strconv"

	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type PostHandler struct {
	svc *service.PostService
}

func NewPostHandler(svc *service.PostService) *PostHandler {
	return &PostHandler{svc: svc}
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	var req dto.CreatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	duplicate, err := h.svc.CheckDuplicateSubmit(c.GetUint("userID"), req.Title, req.Content)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "duplicate submit check failed")
		return
	}
	if duplicate {
		core.Fail(c, http.StatusTooManyRequests, "duplicate post submission")
		return
	}

	post, err := h.svc.CreatePost(c.GetUint("userID"), c.GetString("username"), req.Title, req.Content, req.IsPublic, req.Status, req.PublishAt, req.Topics, req.Images)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, post)
}

func (h *PostHandler) GetPostList(c *gin.Context) {
	var query dto.PostListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.GetPostList(query.Topic, query.Sort, query.Page, query.PageSize)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *PostHandler) GetHotPosts(c *gin.Context) {
	limit := 10
	if rawLimit := c.DefaultQuery("limit", "10"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	data, err := h.svc.GetHotPosts(limit)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, gin.H{"list": data, "limit": limit})
}

func (h *PostHandler) GetPost(c *gin.Context) {
	post, err := h.svc.GetPost(c.Param("id"), c.GetUint("userID"), c.GetString("role"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, post)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	if err := h.svc.DeletePost(c.Param("id"), c.GetUint("userID"), c.GetString("role")); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "delete success", nil)
}

func (h *PostHandler) UpdatePost(c *gin.Context) {
	var req dto.UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.UpdatePost(c.Param("id"), c.GetUint("userID"), req.Title, req.Content, req.IsPublic, req.Status, req.PublishAt, req.Topics, req.Images); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "update success", nil)
}

func (h *PostHandler) GetMyPostList(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		core.Fail(c, http.StatusUnauthorized, "missing user identity")
		return
	}

	posts, err := h.svc.GetMyPostList(userID)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "get my posts success", posts)
}

func (h *PostHandler) GetUserPosts(c *gin.Context) {
	posts, err := h.svc.GetUserPosts(c.Param("id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "get user posts success", posts)
}

func (h *PostHandler) GetUserLikedPosts(c *gin.Context) {
	posts, err := h.svc.GetUserLikedPosts(c.Param("id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "get liked posts success", posts)
}

func (h *PostHandler) GetUserCollectedPosts(c *gin.Context) {
	posts, err := h.svc.GetUserCollectedPosts(c.Param("id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "get collected posts success", posts)
}

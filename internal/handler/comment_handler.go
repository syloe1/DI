package handler

import (
	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type CommentHandler struct {
	svc *service.CommentService
}

func NewCommentHandler(svc *service.CommentService) *CommentHandler {
	return &CommentHandler{svc: svc}
}

func (h *CommentHandler) CreateComment(c *gin.Context) {
	var req dto.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	comment, err := h.svc.CreateComment(req.PostID, c.GetUint("userID"), c.GetString("username"), req.Content, req.ParentID)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, comment)
}

func (h *CommentHandler) DeleteComment(c *gin.Context) {
	if err := h.svc.DeleteComment(c.Param("id"), c.GetUint("userID"), c.GetString("role")); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "delete comment success", nil)
}

func (h *CommentHandler) UpdateComment(c *gin.Context) {
	var req dto.UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.UpdateComment(c.Param("id"), c.GetUint("userID"), req.Content); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "update comment success", nil)
}

func (h *CommentHandler) GetPostComments(c *gin.Context) {
	comments, err := h.svc.GetPostComments(c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, comments)
}

func (h *CommentHandler) GetUserComments(c *gin.Context) {
	comments, err := h.svc.GetUserComments(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, comments)
}

func (h *CommentHandler) GetPublicUserComments(c *gin.Context) {
	comments, err := h.svc.GetPublicUserComments(c.Param("id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, comments)
}

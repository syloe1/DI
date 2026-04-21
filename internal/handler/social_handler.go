package handler

import (
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type SocialHandler struct {
	svc *service.SocialService
}

func NewSocialHandler(svc *service.SocialService) *SocialHandler {
	return &SocialHandler{svc: svc}
}

func (h *SocialHandler) FollowUser(c *gin.Context) {
	data, err := h.svc.FollowUser(c.GetUint("userID"), c.Param("uid"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *SocialHandler) BlockUser(c *gin.Context) {
	data, err := h.svc.BlockUser(c.GetUint("userID"), c.Param("uid"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *SocialHandler) GetRelationStatus(c *gin.Context) {
	data, err := h.svc.GetRelationStatus(c.GetUint("userID"), c.Param("uid"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *SocialHandler) GetFollowList(c *gin.Context) {
	data, err := h.svc.GetFollowList(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *SocialHandler) GetFollowerList(c *gin.Context) {
	data, err := h.svc.GetFollowerList(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *SocialHandler) GetBlockList(c *gin.Context) {
	data, err := h.svc.GetBlockList(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

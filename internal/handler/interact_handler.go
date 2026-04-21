package handler

import (
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type InteractHandler struct {
	svc *service.InteractService
}

func NewInteractHandler(svc *service.InteractService) *InteractHandler {
	return &InteractHandler{svc: svc}
}

func (h *InteractHandler) ToggleLike(c *gin.Context) {
	data, err := h.svc.ToggleLike(c.GetUint("userID"), c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *InteractHandler) ToggleDislike(c *gin.Context) {
	data, err := h.svc.ToggleDislike(c.GetUint("userID"), c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *InteractHandler) ToggleCollect(c *gin.Context) {
	data, err := h.svc.ToggleCollect(c.GetUint("userID"), c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *InteractHandler) Share(c *gin.Context) {
	data, err := h.svc.Share(c.GetUint("userID"), c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *InteractHandler) GetInteractStatus(c *gin.Context) {
	data, err := h.svc.GetInteractStatus(c.GetUint("userID"), c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *InteractHandler) GetInteractCount(c *gin.Context) {
	data, err := h.svc.GetInteractCount(c.Param("post_id"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

package handler

import (
	"errors"
	"net/http"
	"strconv"

	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	err := h.svc.Register(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			core.Fail(c, http.StatusBadRequest, "username already exists")
			return
		}
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "register success", nil)
}

func (h *UserHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	token, role, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, gin.H{
		"token": token,
		"role":  role,
	})
}

func (h *UserHandler) Logout(c *gin.Context) {
	if err := h.svc.Logout(c.GetUint("userID")); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "logout success", nil)
}

func (h *UserHandler) AddUser(c *gin.Context) {
	var req dto.AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	err := h.svc.AddUser(c.GetString("role"), req.Username, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			core.Fail(c, http.StatusBadRequest, "username already exists")
			return
		}
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "add user success", nil)
}

func (h *UserHandler) GetUserList(c *gin.Context) {
	data, err := h.svc.GetUserList(c.GetString("role"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *UserHandler) GetUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	user, err := h.svc.GetUser(id)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, user)
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.UpdateUser(id, c.GetUint("userID"), c.GetString("role"), req.Username, req.Role); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "update success", nil)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.ChangePassword(id, c.GetUint("userID"), c.GetString("role"), req.OldPassword, req.NewPassword); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "password updated", nil)
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteUser(c.GetString("role"), id); err != nil {
		core.FailByError(c, err)
		return
	}
	core.SuccessWithMessage(c, "delete success", nil)
}

func (h *UserHandler) SearchUser(c *gin.Context) {
	data, err := h.svc.SearchUser(c.Query("username"))
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *UserHandler) GetUserOnlineStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	data, err := h.svc.GetUserOnlineStatus(id)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *UserHandler) GetOnlineUsers(c *gin.Context) {
	data, err := h.svc.GetOnlineUsers()
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func (h *UserHandler) BatchGetUserRoles(c *gin.Context) {
	var req dto.BatchGetUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.BatchGetUserRoles(req.IDs)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	core.Success(c, data)
}

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil || id64 == 0 {
		core.Fail(c, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return uint(id64), true
}
